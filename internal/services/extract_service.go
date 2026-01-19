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

// ExtractService handles ZIP extraction operations
type ExtractService struct {
	basePath      string
	progressStore *models.ProgressStore
	owner         string
	uid           int
	gid           int
}

// NewExtractService creates a new extract service
func NewExtractService(basePath string, owner string, progressStore *models.ProgressStore) *ExtractService {
	svc := &ExtractService{
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

// Extract extracts a ZIP archive to the destination
func (s *ExtractService) Extract(source, destination string) (string, error) {
	sourcePath, err := utils.ValidatePath(s.basePath, source)
	if err != nil {
		return "", err
	}

	if !utils.PathExists(sourcePath) {
		return "", ErrNotFound
	}

	destPath, err := utils.ValidatePath(s.basePath, destination)
	if err != nil {
		return "", err
	}

	// Open ZIP file
	zipReader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return "", err
	}
	defer zipReader.Close()

	// Calculate total size for progress
	var totalSize int64
	for _, f := range zipReader.File {
		totalSize += int64(f.UncompressedSize64)
	}

	// Generate extract ID for progress tracking
	extractID := uuid.New().String()

	// Initialize progress
	s.progressStore.Set(extractID, &models.Progress{
		ID:            extractID,
		Filename:      filepath.Base(sourcePath),
		Progress:      0,
		UploadedBytes: 0,
		TotalBytes:    totalSize,
		Status:        models.StatusProcessing,
	})

	// Ensure destination directory exists
	if err := os.MkdirAll(destPath, 0755); err != nil {
		s.updateProgressError(extractID, err.Error())
		return extractID, err
	}

	var extractedBytes int64

	// Extract files
	for _, f := range zipReader.File {
		err := s.extractFile(f, destPath, &extractedBytes, totalSize, extractID)
		if err != nil {
			s.updateProgressError(extractID, err.Error())
			return extractID, err
		}
	}

	s.updateProgressCompleted(extractID)

	relPath, _ := utils.GetRelativePath(s.basePath, destPath)
	return extractID + ":" + relPath, nil
}

// setOwner sets the file owner to the service configured user
func (s *ExtractService) setOwner(path string) error {
	if s.owner == "" {
		return nil
	}
	return utils.SudoChown(path, s.owner)
}

func (s *ExtractService) extractFile(f *zip.File, destPath string, extractedBytes *int64, totalSize int64, progressID string) error {
	// Construct destination path
	filePath := filepath.Join(destPath, f.Name)

	// Security check: prevent path traversal
	if !filepath.HasPrefix(filePath, filepath.Clean(destPath)+string(os.PathSeparator)) {
		return utils.ErrPathTraversal
	}

	if f.FileInfo().IsDir() {
		if err := os.MkdirAll(filePath, f.Mode()); err != nil {
			return err
		}
		return s.setOwner(filePath)
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	// Note: We might want to set owner for parent directories too, but usually it's recursive from top level call or expected to exist.

	// Open source file from ZIP
	srcFile, err := f.Open()
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create destination file
	dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	// Defer close first
	defer dstFile.Close()

	// Copy with progress tracking
	buf := make([]byte, utils.DefaultBufferSize)
	for {
		n, err := srcFile.Read(buf)
		if n > 0 {
			if _, werr := dstFile.Write(buf[:n]); werr != nil {
				return werr
			}
			newVal := atomic.AddInt64(extractedBytes, int64(n))
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

	// Set owner
	s.setOwner(filePath)

	return nil
}

// GetProgress returns progress for an extraction operation
func (s *ExtractService) GetProgress(extractID string) (*models.Progress, bool) {
	return s.progressStore.Get(extractID)
}

func (s *ExtractService) updateProgressError(extractID, errorMsg string) {
	if p, ok := s.progressStore.Get(extractID); ok {
		p.Status = models.StatusFailed
		p.Error = errorMsg
		s.progressStore.Set(extractID, p)
	}
}

func (s *ExtractService) updateProgressCompleted(extractID string) {
	if p, ok := s.progressStore.Get(extractID); ok {
		p.Status = models.StatusCompleted
		p.Progress = 100
		p.UploadedBytes = p.TotalBytes
		s.progressStore.Set(extractID, p)
	}
}
