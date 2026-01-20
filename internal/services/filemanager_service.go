package services

import (
	"errors"
	"filemanager-api/internal/models"
	"filemanager-api/internal/utils"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var (
	ErrNotFound         = errors.New("file or folder not found")
	ErrAlreadyExists    = errors.New("file or folder already exists")
	ErrNotAFile         = errors.New("path is not a file")
	ErrNotAFolder       = errors.New("path is not a folder")
	ErrFolderNotEmpty   = errors.New("folder is not empty")
	ErrPermissionDenied = errors.New("permission denied")
	ErrSSHConnection    = errors.New("SSH connection failed")
)

// SSHConfig holds SSH connection details
type SSHConfig struct {
	Host       string
	Port       string
	Username   string
	PrivateKey string
}

// FileManagerService handles all file and folder operations
type FileManagerService struct {
	basePath   string
	sshConfig  *SSHConfig
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	isRemote   bool
	owner      string
	uid        int
	gid        int
}

// NewFileManagerService creates a new file manager service for local operations
func NewFileManagerService(basePath string, owner string) *FileManagerService {
	svc := &FileManagerService{
		basePath: basePath,
		isRemote: false,
		owner:    owner,
		uid:      -1, // Default to no change if lookup fails
		gid:      -1,
	}

	if owner != "" {
		uid, gid, err := utils.ResolveUser(owner)
		if err == nil {
			svc.uid = uid
			svc.gid = gid
			fmt.Printf("[INFO] Ownership resolved: %s -> UID:%d, GID:%d\n", owner, svc.uid, svc.gid)
		} else {
			fmt.Printf("[ERROR] Failed to resolve user %s: %v. Files will be owned by root.\n", owner, err)
		}
	} else {
		fmt.Printf("[WARN] No owner specified for FileManagerService\n")
	}

	return svc
}

// NewRemoteFileManagerService creates a new file manager service for remote SSH operations
func NewRemoteFileManagerService(basePath string, sshConfig *SSHConfig, owner string) (*FileManagerService, error) {
	svc := &FileManagerService{
		basePath:  basePath,
		sshConfig: sshConfig,
		isRemote:  true,
		owner:     owner,
	}

	if err := svc.connectSSH(); err != nil {
		return nil, err
	}

	if owner != "" {
		fmt.Printf("[INFO] Remote service with ownership: %s\n", owner)
	}

	return svc, nil
}

// connectSSH establishes SSH and SFTP connections
func (s *FileManagerService) connectSSH() error {
	signer, err := ssh.ParsePrivateKey([]byte(s.sshConfig.PrivateKey))
	if err != nil {
		return fmt.Errorf("%w: failed to parse private key: %v", ErrSSHConnection, err)
	}

	config := &ssh.ClientConfig{
		User: s.sshConfig.Username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // In production, use known_hosts
	}

	addr := fmt.Sprintf("%s:%s", s.sshConfig.Host, s.sshConfig.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSSHConnection, err)
	}
	s.sshClient = client

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return fmt.Errorf("%w: failed to create SFTP client: %v", ErrSSHConnection, err)
	}
	s.sftpClient = sftpClient

	return nil
}

// Close closes SSH connections
func (s *FileManagerService) Close() {
	if s.sftpClient != nil {
		s.sftpClient.Close()
	}
	if s.sshClient != nil {
		s.sshClient.Close()
	}
}

// IsRemote returns true if this is a remote connection
func (s *FileManagerService) IsRemote() bool {
	return s.isRemote
}

// GetFullPath validates and returns the full path for a relative path
func (s *FileManagerService) GetFullPath(relativePath string) (string, error) {
	return utils.ValidatePath(s.basePath, relativePath)
}

// runSSHCommand executes a command on the remote server via SSH
func (s *FileManagerService) runSSHCommand(cmd string) error {
	if s.sshClient == nil {
		return fmt.Errorf("SSH client not connected")
	}

	session, err := s.sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("SSH command failed: %v, output: %s", err, string(output))
	}
	return nil
}

// setOwner sets the file owner to the service configured user
func (s *FileManagerService) setOwner(path string) error {
	fmt.Printf("[DEBUG] setOwner called: path=%s, owner=%s, isRemote=%v\n", path, s.owner, s.isRemote)

	if s.owner == "" {
		fmt.Printf("[WARN] setOwner: owner is empty, skipping\n")
		return nil
	}

	if s.isRemote {
		// Execute chown via SSH
		cmd := fmt.Sprintf("chown %s:%s %s", s.owner, s.owner, path)
		fmt.Printf("[DEBUG] Running SSH chown: %s\n", cmd)
		err := s.runSSHCommand(cmd)
		if err != nil {
			fmt.Printf("[ERROR] SSH chown failed: %v\n", err)
		}
		return err
	}

	// Local: use chown command
	fmt.Printf("[DEBUG] Running local chown: chown %s:%s %s\n", s.owner, s.owner, path)
	err := utils.SudoChown(path, s.owner)
	if err != nil {
		fmt.Printf("[ERROR] Local chown failed: %v\n", err)
	}
	return err
}

// setOwnerRecursive sets the file owner recursively
func (s *FileManagerService) setOwnerRecursive(path string) error {
	if s.owner == "" {
		return nil
	}

	if s.isRemote {
		// Execute chown -R via SSH
		cmd := fmt.Sprintf("chown -R %s:%s %s", s.owner, s.owner, path)
		return s.runSSHCommand(cmd)
	}

	// Local: use chown -R command
	return utils.SudoChownRecursive(path, s.owner)
}

// List lists all files and folders in a directory
func (s *FileManagerService) List(relativePath string) ([]models.FileInfo, error) {
	fullPath, err := utils.ValidatePath(s.basePath, relativePath)
	if err != nil {
		return nil, err
	}

	var items []models.FileInfo

	if s.isRemote {
		items, err = s.listRemote(fullPath)
	} else {
		items, err = s.listLocal(fullPath)
	}

	if err != nil {
		return nil, err
	}

	// Sort: folders first, then files, alphabetically
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	return items, nil
}

func (s *FileManagerService) listLocal(fullPath string) ([]models.FileInfo, error) {
	if !utils.IsDir(fullPath) {
		return nil, ErrNotAFolder
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	var items []models.FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		entryPath := filepath.Join(fullPath, entry.Name())
		relPath, _ := utils.GetRelativePath(s.basePath, entryPath)

		item := models.FileInfo{
			Name:        entry.Name(),
			Path:        relPath,
			Size:        info.Size(),
			IsDir:       entry.IsDir(),
			Mode:        info.Mode(),
			ModTime:     info.ModTime(),
			Permissions: utils.FormatPermissions(info.Mode()),
		}

		if !entry.IsDir() {
			item.Extension = strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
			item.MimeType = utils.GetMimeType(entry.Name())
		}

		items = append(items, item)
	}

	return items, nil
}

func (s *FileManagerService) listRemote(fullPath string) ([]models.FileInfo, error) {
	info, err := s.sftpClient.Stat(fullPath)
	if err != nil {
		return nil, ErrNotFound
	}
	if !info.IsDir() {
		return nil, ErrNotAFolder
	}

	entries, err := s.sftpClient.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	var items []models.FileInfo
	for _, entry := range entries {
		entryPath := filepath.Join(fullPath, entry.Name())
		relPath, _ := utils.GetRelativePath(s.basePath, entryPath)

		item := models.FileInfo{
			Name:        entry.Name(),
			Path:        relPath,
			Size:        entry.Size(),
			IsDir:       entry.IsDir(),
			Mode:        entry.Mode(),
			ModTime:     entry.ModTime(),
			Permissions: utils.FormatPermissions(entry.Mode()),
		}

		if !entry.IsDir() {
			item.Extension = strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
			item.MimeType = utils.GetMimeType(entry.Name())
		}

		items = append(items, item)
	}

	return items, nil
}

// GetInfo gets file or folder information
func (s *FileManagerService) GetInfo(relativePath string) (*models.FileInfo, error) {
	fullPath, err := utils.ValidatePath(s.basePath, relativePath)
	if err != nil {
		return nil, err
	}

	if s.isRemote {
		return s.getInfoRemote(fullPath)
	}
	return s.getInfoLocal(fullPath)
}

func (s *FileManagerService) getInfoLocal(fullPath string) (*models.FileInfo, error) {
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	relPath, _ := utils.GetRelativePath(s.basePath, fullPath)

	item := &models.FileInfo{
		Name:        info.Name(),
		Path:        relPath,
		Size:        info.Size(),
		IsDir:       info.IsDir(),
		Mode:        info.Mode(),
		ModTime:     info.ModTime(),
		Permissions: utils.FormatPermissions(info.Mode()),
	}

	if !info.IsDir() {
		item.Extension = strings.TrimPrefix(filepath.Ext(info.Name()), ".")
		item.MimeType = utils.GetMimeType(info.Name())
	} else {
		size, _ := utils.GetDirectorySize(fullPath)
		item.Size = size
	}

	return item, nil
}

func (s *FileManagerService) getInfoRemote(fullPath string) (*models.FileInfo, error) {
	info, err := s.sftpClient.Stat(fullPath)
	if err != nil {
		return nil, ErrNotFound
	}

	relPath, _ := utils.GetRelativePath(s.basePath, fullPath)

	item := &models.FileInfo{
		Name:        info.Name(),
		Path:        relPath,
		Size:        info.Size(),
		IsDir:       info.IsDir(),
		Mode:        info.Mode(),
		ModTime:     info.ModTime(),
		Permissions: utils.FormatPermissions(info.Mode()),
	}

	if !info.IsDir() {
		item.Extension = strings.TrimPrefix(filepath.Ext(info.Name()), ".")
		item.MimeType = utils.GetMimeType(info.Name())
	}

	return item, nil
}

// GetContent reads file content
func (s *FileManagerService) GetContent(relativePath string) (io.ReadCloser, *models.FileInfo, error) {
	fullPath, err := utils.ValidatePath(s.basePath, relativePath)
	if err != nil {
		return nil, nil, err
	}

	info, err := s.GetInfo(relativePath)
	if err != nil {
		return nil, nil, err
	}

	if info.IsDir {
		return nil, nil, ErrNotAFile
	}

	if s.isRemote {
		file, err := s.sftpClient.Open(fullPath)
		if err != nil {
			return nil, nil, err
		}
		return file, info, nil
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, nil, err
	}
	return file, info, nil
}

// CreateFile creates a new file with content
func (s *FileManagerService) CreateFile(relativePath string, content string) (*models.FileInfo, error) {
	fullPath, err := utils.ValidatePath(s.basePath, relativePath)
	if err != nil {
		return nil, err
	}

	if s.isRemote {
		return s.createFileRemote(fullPath, relativePath, content)
	}
	return s.createFileLocal(fullPath, relativePath, content)
}

func (s *FileManagerService) createFileLocal(fullPath, relativePath, content string) (*models.FileInfo, error) {
	if utils.PathExists(fullPath) {
		return nil, ErrAlreadyExists
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return nil, err
	}

	// Set owner
	if err := s.setOwner(fullPath); err != nil {
		// Log error but continue
		fmt.Printf("Failed to set owner for %s: %v\n", fullPath, err)
	}

	return s.GetInfo(relativePath)
}

func (s *FileManagerService) createFileRemote(fullPath, relativePath, content string) (*models.FileInfo, error) {
	_, err := s.sftpClient.Stat(fullPath)
	if err == nil {
		return nil, ErrAlreadyExists
	}

	dir := filepath.Dir(fullPath)
	s.sftpClient.MkdirAll(dir)

	file, err := s.sftpClient.Create(fullPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if _, err := file.Write([]byte(content)); err != nil {
		return nil, err
	}

	// Set owner via SSH
	if err := s.setOwner(fullPath); err != nil {
		fmt.Printf("Failed to set owner for %s: %v\n", fullPath, err)
	}

	return s.GetInfo(relativePath)
}

// UpdateFile updates an existing file's content
func (s *FileManagerService) UpdateFile(relativePath string, content string) (*models.FileInfo, error) {
	fullPath, err := utils.ValidatePath(s.basePath, relativePath)
	if err != nil {
		return nil, err
	}

	if s.isRemote {
		return s.updateFileRemote(fullPath, relativePath, content)
	}
	return s.updateFileLocal(fullPath, relativePath, content)
}

func (s *FileManagerService) updateFileLocal(fullPath, relativePath, content string) (*models.FileInfo, error) {
	if !utils.PathExists(fullPath) {
		return nil, ErrNotFound
	}

	if utils.IsDir(fullPath) {
		return nil, ErrNotAFile
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return nil, err
	}

	// Set owner (ensure owner stays correct)
	if err := s.setOwner(fullPath); err != nil {
		fmt.Printf("Failed to set owner for %s: %v\n", fullPath, err)
	}

	return s.GetInfo(relativePath)
}

func (s *FileManagerService) updateFileRemote(fullPath, relativePath, content string) (*models.FileInfo, error) {
	info, err := s.sftpClient.Stat(fullPath)
	if err != nil {
		return nil, ErrNotFound
	}

	if info.IsDir() {
		return nil, ErrNotAFile
	}

	file, err := s.sftpClient.Create(fullPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if _, err := file.Write([]byte(content)); err != nil {
		return nil, err
	}

	// Set owner via SSH
	if err := s.setOwner(fullPath); err != nil {
		fmt.Printf("Failed to set owner for %s: %v\n", fullPath, err)
	}

	return s.GetInfo(relativePath)
}

// CreateFolder creates a new folder
func (s *FileManagerService) CreateFolder(relativePath string) (*models.FileInfo, error) {
	fullPath, err := utils.ValidatePath(s.basePath, relativePath)
	if err != nil {
		return nil, err
	}

	if s.isRemote {
		_, statErr := s.sftpClient.Stat(fullPath)
		if statErr == nil {
			return nil, ErrAlreadyExists
		}
		if err := s.sftpClient.MkdirAll(fullPath); err != nil {
			return nil, err
		}
		// Set owner via SSH
		if err := s.setOwner(fullPath); err != nil {
			fmt.Printf("Failed to set owner for %s: %v\n", fullPath, err)
		}
	} else {
		if utils.PathExists(fullPath) {
			return nil, ErrAlreadyExists
		}
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			return nil, err
		}
		if err := s.setOwner(fullPath); err != nil {
			fmt.Printf("Failed to set owner for %s: %v\n", fullPath, err)
		}
	}

	return s.GetInfo(relativePath)
}

// Rename renames a file or folder
func (s *FileManagerService) Rename(relativePath, newName string) (*models.FileInfo, error) {
	fullPath, err := utils.ValidatePath(s.basePath, relativePath)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(fullPath)
	newPath := filepath.Join(dir, newName)

	if s.isRemote {
		if _, err := s.sftpClient.Stat(fullPath); err != nil {
			return nil, ErrNotFound
		}
		if _, err := s.sftpClient.Stat(newPath); err == nil {
			return nil, ErrAlreadyExists
		}
		if err := s.sftpClient.Rename(fullPath, newPath); err != nil {
			return nil, err
		}
	} else {
		if !utils.PathExists(fullPath) {
			return nil, ErrNotFound
		}
		if utils.PathExists(newPath) {
			return nil, ErrAlreadyExists
		}
		if err := os.Rename(fullPath, newPath); err != nil {
			return nil, err
		}
	}

	newRelPath, _ := utils.GetRelativePath(s.basePath, newPath)
	return s.GetInfo(newRelPath)
}

// Delete deletes a file or folder
func (s *FileManagerService) Delete(relativePath string, recursive bool) error {
	fmt.Printf("[DEBUG] Delete: relativePath=%s, basePath=%s\n", relativePath, s.basePath)

	fullPath, err := utils.ValidatePath(s.basePath, relativePath)
	if err != nil {
		fmt.Printf("[ERROR] Delete: ValidatePath error: %v\n", err)
		return err
	}

	fmt.Printf("[DEBUG] Delete: fullPath=%s, isRemote=%v\n", fullPath, s.isRemote)

	if s.isRemote {
		return s.deleteRemote(fullPath, recursive)
	}
	return s.deleteLocal(fullPath, recursive)
}

func (s *FileManagerService) deleteLocal(fullPath string, recursive bool) error {
	if !utils.PathExists(fullPath) {
		return ErrNotFound
	}

	if utils.IsDir(fullPath) {
		if !recursive {
			entries, err := os.ReadDir(fullPath)
			if err != nil {
				return err
			}
			if len(entries) > 0 {
				return ErrFolderNotEmpty
			}
			return os.Remove(fullPath)
		}
		return os.RemoveAll(fullPath)
	}

	return os.Remove(fullPath)
}

func (s *FileManagerService) deleteRemote(fullPath string, recursive bool) error {
	info, err := s.sftpClient.Stat(fullPath)
	if err != nil {
		return ErrNotFound
	}

	if info.IsDir() {
		if !recursive {
			entries, err := s.sftpClient.ReadDir(fullPath)
			if err != nil {
				return err
			}
			if len(entries) > 0 {
				return ErrFolderNotEmpty
			}
			return s.sftpClient.RemoveDirectory(fullPath)
		}
		return s.removeAllRemote(fullPath)
	}

	return s.sftpClient.Remove(fullPath)
}

func (s *FileManagerService) removeAllRemote(path string) error {
	entries, err := s.sftpClient.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			if err := s.removeAllRemote(entryPath); err != nil {
				return err
			}
		} else {
			if err := s.sftpClient.Remove(entryPath); err != nil {
				return err
			}
		}
	}

	return s.sftpClient.RemoveDirectory(path)
}

// Copy copies files/folders to destination
func (s *FileManagerService) Copy(sources []string, destination string, overwrite bool) ([]models.FileInfo, error) {
	destPath, err := utils.ValidatePath(s.basePath, destination)
	if err != nil {
		return nil, err
	}

	if s.isRemote {
		s.sftpClient.MkdirAll(destPath)
	} else {
		if err := os.MkdirAll(destPath, 0755); err != nil {
			return nil, err
		}
	}

	var copied []models.FileInfo

	for _, src := range sources {
		srcPath, err := utils.ValidatePath(s.basePath, src)
		if err != nil {
			return nil, err
		}

		var srcInfo os.FileInfo
		if s.isRemote {
			srcInfo, err = s.sftpClient.Stat(srcPath)
		} else {
			srcInfo, err = os.Stat(srcPath)
		}
		if err != nil {
			continue
		}

		dstItem := filepath.Join(destPath, srcInfo.Name())

		if s.isRemote {
			if _, err := s.sftpClient.Stat(dstItem); err == nil && !overwrite {
				dstItem = utils.GenerateUniqueName(dstItem)
			}
		} else {
			if utils.PathExists(dstItem) && !overwrite {
				dstItem = utils.GenerateUniqueName(dstItem)
			}
		}

		if srcInfo.IsDir() {
			if s.isRemote {
				if err := s.copyDirRemote(srcPath, dstItem); err != nil {
					return nil, err
				}
			} else {
				if err := utils.CopyDir(srcPath, dstItem, true); err != nil {
					return nil, err
				}
				// Recursive set owner for copied folder
				if err := s.setOwnerRecursive(dstItem); err != nil {
					fmt.Printf("Failed to set owner for %s: %v\n", dstItem, err)
				}
			}
		} else {
			if s.isRemote {
				if err := s.copyFileRemote(srcPath, dstItem); err != nil {
					return nil, err
				}
			} else {
				if err := utils.CopyFile(srcPath, dstItem, true); err != nil {
					return nil, err
				}
				// Set owner for copied file
				if err := s.setOwner(dstItem); err != nil {
					fmt.Printf("Failed to set owner for %s: %v\n", dstItem, err)
				}
			}
		}

		relPath, _ := utils.GetRelativePath(s.basePath, dstItem)
		info, _ := s.GetInfo(relPath)
		if info != nil {
			copied = append(copied, *info)
		}
	}

	return copied, nil
}

func (s *FileManagerService) copyFileRemote(src, dst string) error {
	srcFile, err := s.sftpClient.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := s.sftpClient.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func (s *FileManagerService) copyDirRemote(src, dst string) error {
	s.sftpClient.MkdirAll(dst)
	
	entries, err := s.sftpClient.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := s.copyDirRemote(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := s.copyFileRemote(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// runSSHCommandOutput executes a command on the remote server via SSH and returns output
func (s *FileManagerService) runSSHCommandOutput(cmd string) ([]byte, error) {
	if s.sshClient == nil {
		return nil, fmt.Errorf("SSH client not connected")
	}

	session, err := s.sshClient.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	return session.CombinedOutput(cmd)
}

// GetDiskUsage calculates the total size of a file or directory
func (s *FileManagerService) GetDiskUsage(relativePath string) (int64, error) {
	fullPath, err := utils.ValidatePath(s.basePath, relativePath)
	if err != nil {
		return 0, err
	}

	if s.isRemote {
		// Use du -sb for remote calculation (much faster than recursive sftp)
		cmd := fmt.Sprintf("du -sb '%s' | awk '{print $1}'", fullPath)
		output, err := s.runSSHCommandOutput(cmd)
		if err != nil {
			return 0, fmt.Errorf("remote disk usage check failed: %v", err)
		}
		
		sizeStr := strings.TrimSpace(string(output))
		// Handle potential errors in output that aren't exit codes
		if !isNumeric(sizeStr) {
			return 0, fmt.Errorf("unexpected output from du: %s", sizeStr)
		}
		
		return strconv.ParseInt(sizeStr, 10, 64)
	}

	// Local calculation
	return utils.GetDirectorySize(fullPath)
}

func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}




// Move moves files/folders to destination
func (s *FileManagerService) Move(sources []string, destination string, overwrite bool) ([]models.FileInfo, error) {
	destPath, err := utils.ValidatePath(s.basePath, destination)
	if err != nil {
		return nil, err
	}

	if s.isRemote {
		s.sftpClient.MkdirAll(destPath)
	} else {
		if err := os.MkdirAll(destPath, 0755); err != nil {
			return nil, err
		}
	}

	var moved []models.FileInfo

	for _, src := range sources {
		srcPath, err := utils.ValidatePath(s.basePath, src)
		if err != nil {
			return nil, err
		}

		var srcInfo os.FileInfo
		if s.isRemote {
			srcInfo, err = s.sftpClient.Stat(srcPath)
		} else {
			srcInfo, err = os.Stat(srcPath)
		}
		if err != nil {
			continue
		}

		dstItem := filepath.Join(destPath, srcInfo.Name())

		if s.isRemote {
			if _, err := s.sftpClient.Stat(dstItem); err == nil && !overwrite {
				dstItem = utils.GenerateUniqueName(dstItem)
			}
			if err := s.sftpClient.Rename(srcPath, dstItem); err != nil {
				// Fallback to copy + delete
				if srcInfo.IsDir() {
					if err := s.copyDirRemote(srcPath, dstItem); err != nil {
						return nil, err
					}
					s.removeAllRemote(srcPath)
				} else {
					if err := s.copyFileRemote(srcPath, dstItem); err != nil {
						return nil, err
					}
					s.sftpClient.Remove(srcPath)
				}
			}
		} else {
			if utils.PathExists(dstItem) && !overwrite {
				dstItem = utils.GenerateUniqueName(dstItem)
			}
			if err := os.Rename(srcPath, dstItem); err != nil {
				if srcInfo.IsDir() {
					if err := utils.CopyDir(srcPath, dstItem, true); err != nil {
						return nil, err
					}
					os.RemoveAll(srcPath)
					s.setOwnerRecursive(dstItem)
				} else {
					if err := utils.CopyFile(srcPath, dstItem, true); err != nil {
						return nil, err
					}
					os.Remove(srcPath)
					s.setOwner(dstItem)
				}
			} else {
				// Rename successful, enforce ownership
				if srcInfo.IsDir() {
					s.setOwnerRecursive(dstItem)
				} else {
					s.setOwner(dstItem)
				}
			}
		}

		relPath, _ := utils.GetRelativePath(s.basePath, dstItem)
		info, _ := s.GetInfo(relPath)
		if info != nil {
			moved = append(moved, *info)
		}
	}

	return moved, nil
}
