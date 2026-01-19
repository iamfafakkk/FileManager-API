package services

import (
	"archive/zip"
	"filemanager-api/internal/models"
	"filemanager-api/internal/utils"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/google/uuid"
)

// CompressService handles file compression operations
type CompressService struct {
	basePath      string
	progressStore *models.ProgressStore
	owner         string
	uid           int
	gid           int
}

// NewCompressService creates a new compress service
func NewCompressService(basePath string, owner string, progressStore *models.ProgressStore) *CompressService {
	svc := &CompressService{
		basePath:      basePath,
		progressStore: progressStore,
		owner:         owner,
		uid:           -1,
		gid:           -1,
	}

	if owner != "" {
		uid, gid, err := utils.ResolveUser(owner)
		if err == nil {
			svc.uid = uid
			svc.gid = gid
		} else {
			fmt.Printf("[ERROR] Failed to resolve user %s: %v\n", owner, err)
		}
	}

	return svc
}

// setOwner sets the file owner to the service configured user
func (s *CompressService) setOwner(path string) error {
	if s.owner == "" {
		return nil
	}
	return utils.SudoChown(path, s.owner)
}

// Compress creates a ZIP archive from the given paths
func (s *CompressService) Compress(paths []string, output string, compressionLevel int) (string, error) {
	outputPath, err := utils.ValidatePath(s.basePath, output)
	if err != nil {
		return "", err
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return "", err
	}

	// Generate unique name if file exists
	if utils.PathExists(outputPath) {
		outputPath = utils.GenerateUniqueName(outputPath)
	}

	// Calculate total size for progress
	var totalSize int64
	validPaths := make([]string, 0)

	for _, p := range paths {
		fullPath, err := utils.ValidatePath(s.basePath, p)
		if err != nil {
			continue
		}
		if !utils.PathExists(fullPath) {
			continue
		}

		validPaths = append(validPaths, fullPath)

		if utils.IsDir(fullPath) {
			size, _ := utils.GetDirectorySize(fullPath)
			totalSize += size
		} else {
			info, _ := os.Stat(fullPath)
			totalSize += info.Size()
		}
	}

	if len(validPaths) == 0 {
		return "", ErrNotFound
	}

	// Generate compress ID for progress tracking
	compressID := uuid.New().String()

	// Initialize progress
	s.progressStore.Set(compressID, &models.Progress{
		ID:            compressID,
		Filename:      filepath.Base(outputPath),
		Progress:      0,
		UploadedBytes: 0,
		TotalBytes:    totalSize,
		Status:        models.StatusProcessing,
	})

	// Create ZIP file
	zipFile, err := os.Create(outputPath)
	if err != nil {
		s.updateProgressError(compressID, err.Error())
		return compressID, err
	}
	// Defer close using closure to handle error logic if needed, but structure requires simple defer.
	// We will chown after close if possible, but we can only chown by path after creation.
	// Actually os.Chown works on file path even if open.
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)

	// We must close zipWriter to flush data
	// Defer LIFO: zipWriter.Close() runs first, then zipFile.Close()
	defer zipWriter.Close()

	// Track compressed bytes
	var compressedBytes int64

	// Add files to archive
	for _, fullPath := range validPaths {
		if utils.IsDir(fullPath) {
			err = s.addDirectoryToZip(zipWriter, fullPath, filepath.Base(fullPath), &compressedBytes, totalSize, compressID)
		} else {
			err = s.addFileToZip(zipWriter, fullPath, filepath.Base(fullPath), &compressedBytes, totalSize, compressID)
		}
		if err != nil {
			s.updateProgressError(compressID, err.Error())
			return compressID, err
		}
	}

	// Set owner of the zip file
	s.setOwner(outputPath)

	s.updateProgressCompleted(compressID)

	relPath, _ := utils.GetRelativePath(s.basePath, outputPath)
	return compressID + ":" + relPath, nil
}

func (s *CompressService) addFileToZip(zipWriter *zip.Writer, filePath, zipPath string, compressedBytes *int64, totalSize int64, progressID string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	header.Name = zipPath
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	// Copy with progress tracking
	buf := make([]byte, utils.DefaultBufferSize)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			if _, werr := writer.Write(buf[:n]); werr != nil {
				return werr
			}
			newVal := atomic.AddInt64(compressedBytes, int64(n))
			if totalSize > 0 {
				progress := int((newVal * 100) / totalSize)
				if p, ok := s.progressStore.Get(progressID); ok {
					p.Progress = progress
					p.UploadedBytes = newVal
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *CompressService) addDirectoryToZip(zipWriter *zip.Writer, dirPath, zipPath string, compressedBytes *int64, totalSize int64, progressID string) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}

		entryPath := filepath.Join(zipPath, relPath)

		if info.IsDir() {
			// Add directory entry
			_, err := zipWriter.Create(entryPath + "/")
			return err
		}

		return s.addFileToZip(zipWriter, path, entryPath, compressedBytes, totalSize, progressID)
	})
}

// GetProgress returns progress for a compression operation
func (s *CompressService) GetProgress(compressID string) (*models.Progress, bool) {
	return s.progressStore.Get(compressID)
}

func (s *CompressService) updateProgressError(compressID, errorMsg string) {
	if p, ok := s.progressStore.Get(compressID); ok {
		p.Status = models.StatusFailed
		p.Error = errorMsg
		s.progressStore.Set(compressID, p)
	}
}

func (s *CompressService) updateProgressCompleted(compressID string) {
	if p, ok := s.progressStore.Get(compressID); ok {
		p.Status = models.StatusCompleted
		p.Progress = 100
		p.UploadedBytes = p.TotalBytes
		s.progressStore.Set(compressID, p)
	}
}
