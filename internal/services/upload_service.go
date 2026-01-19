package services

import (
	"filemanager-api/internal/models"
	"filemanager-api/internal/utils"
	"filemanager-api/pkg/progresswriter"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
)

// UploadService handles file upload operations
type UploadService struct {
	basePath      string
	progressStore *models.ProgressStore
	chunkStore    *ChunkStore
	owner         string
	uid           int
	gid           int
}

// ChunkStore stores pending chunked uploads
type ChunkStore struct {
	mu     sync.RWMutex
	chunks map[string]*ChunkUpload
}

// ChunkUpload represents a pending chunked upload
type ChunkUpload struct {
	ID          string
	Filename    string
	Destination string
	TotalSize   int64
	ChunkSize   int
	TotalChunks int
	Chunks      map[int]bool
	TempDir     string
}

// NewUploadService creates a new upload service
func NewUploadService(basePath string, owner string, progressStore *models.ProgressStore) *UploadService {
	svc := &UploadService{
		basePath:      basePath,
		progressStore: progressStore,
		chunkStore: &ChunkStore{
			chunks: make(map[string]*ChunkUpload),
		},
		owner: owner,
		uid:   -1,
		gid:   -1,
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
func (s *UploadService) setOwner(path string) error {
	if s.owner == "" {
		return nil
	}
	return utils.SudoChown(path, s.owner)
}

// Upload handles a single file upload with progress tracking
func (s *UploadService) Upload(filename, destination string, reader io.Reader, size int64) (string, error) {
	destPath, err := utils.ValidatePath(s.basePath, destination)
	if err != nil {
		return "", err
	}

	// Ensure destination directory exists
	// Note: We might want chown on created dirs too, but usually destination exists
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return "", err
	}

	fullPath := filepath.Join(destPath, filename)

	// Generate unique name if file exists
	if utils.PathExists(fullPath) {
		fullPath = utils.GenerateUniqueName(fullPath)
	}

	// Generate upload ID for progress tracking
	uploadID := uuid.New().String()

	// Initialize progress
	s.progressStore.Set(uploadID, &models.Progress{
		ID:            uploadID,
		Filename:      filepath.Base(fullPath),
		Progress:      0,
		UploadedBytes: 0,
		TotalBytes:    size,
		Status:        models.StatusUploading,
	})

	// Create destination file
	file, err := os.Create(fullPath)
	if err != nil {
		s.updateProgressError(uploadID, err.Error())
		return uploadID, err
	}

	// Ensure file is closed before marking completion or returning
	// Use function closure for safe usage of file variable which might be reused or not needed if we want cleaner code
	// But minimal change: keep structure.

	// We need ownership set after creation.
	// os.Create opens the file. We can fchown if we want, but os.Chown by path is fine.

	defer file.Close()

	// Create progress writer
	pw := progresswriter.NewProgressWriter(file, size, func(written, total int64) {
		s.progressStore.Update(uploadID, written)
	})

	// Copy with buffer
	buf := make([]byte, utils.DefaultBufferSize)
	_, err = io.CopyBuffer(pw, reader, buf)
	if err != nil {
		s.updateProgressError(uploadID, err.Error())
		return uploadID, err
	}

	// Set owner
	s.setOwner(fullPath)

	// Mark as completed
	s.updateProgressCompleted(uploadID)

	return uploadID, nil
}

// InitChunkedUpload initializes a chunked upload session
func (s *UploadService) InitChunkedUpload(filename, destination string, totalSize int64, chunkSize int) (*ChunkUpload, error) {
	destPath, err := utils.ValidatePath(s.basePath, destination)
	if err != nil {
		return nil, err
	}

	uploadID := uuid.New().String()
	totalChunks := int((totalSize + int64(chunkSize) - 1) / int64(chunkSize))

	// Create temp directory for chunks
	tempDir := filepath.Join(os.TempDir(), "filemanager-chunks", uploadID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, err
	}

	chunk := &ChunkUpload{
		ID:          uploadID,
		Filename:    filename,
		Destination: destPath,
		TotalSize:   totalSize,
		ChunkSize:   chunkSize,
		TotalChunks: totalChunks,
		Chunks:      make(map[int]bool),
		TempDir:     tempDir,
	}

	s.chunkStore.mu.Lock()
	s.chunkStore.chunks[uploadID] = chunk
	s.chunkStore.mu.Unlock()

	// Initialize progress
	s.progressStore.Set(uploadID, &models.Progress{
		ID:            uploadID,
		Filename:      filename,
		Progress:      0,
		UploadedBytes: 0,
		TotalBytes:    totalSize,
		Status:        models.StatusPending,
	})

	return chunk, nil
}

// UploadChunk uploads a single chunk
func (s *UploadService) UploadChunk(uploadID string, chunkIndex int, data []byte) error {
	s.chunkStore.mu.RLock()
	chunk, ok := s.chunkStore.chunks[uploadID]
	s.chunkStore.mu.RUnlock()

	if !ok {
		return ErrNotFound
	}

	// Write chunk to temp file
	chunkPath := filepath.Join(chunk.TempDir, string(rune('0'+chunkIndex)))
	if err := os.WriteFile(chunkPath, data, 0644); err != nil {
		return err
	}

	s.chunkStore.mu.Lock()
	chunk.Chunks[chunkIndex] = true
	uploadedChunks := len(chunk.Chunks)
	s.chunkStore.mu.Unlock()

	// Update progress
	uploadedBytes := int64(uploadedChunks * chunk.ChunkSize)
	if uploadedBytes > chunk.TotalSize {
		uploadedBytes = chunk.TotalSize
	}
	s.progressStore.Update(uploadID, uploadedBytes)

	// Check if all chunks are uploaded
	if uploadedChunks == chunk.TotalChunks {
		return s.finalizeChunkedUpload(uploadID)
	}

	return nil
}

// finalizeChunkedUpload assembles chunks into final file
func (s *UploadService) finalizeChunkedUpload(uploadID string) error {
	s.chunkStore.mu.Lock()
	chunk, ok := s.chunkStore.chunks[uploadID]
	if !ok {
		s.chunkStore.mu.Unlock()
		return ErrNotFound
	}
	delete(s.chunkStore.chunks, uploadID)
	s.chunkStore.mu.Unlock()

	// Create final file
	finalPath := filepath.Join(chunk.Destination, chunk.Filename)
	if utils.PathExists(finalPath) {
		finalPath = utils.GenerateUniqueName(finalPath)
	}

	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		s.updateProgressError(uploadID, err.Error())
		return err
	}

	file, err := os.Create(finalPath)
	if err != nil {
		s.updateProgressError(uploadID, err.Error())
		return err
	}
	defer file.Close()

	// Assemble chunks
	for i := 0; i < chunk.TotalChunks; i++ {
		chunkPath := filepath.Join(chunk.TempDir, string(rune('0'+i)))
		chunkData, err := os.ReadFile(chunkPath)
		if err != nil {
			s.updateProgressError(uploadID, err.Error())
			return err
		}
		if _, err := file.Write(chunkData); err != nil {
			s.updateProgressError(uploadID, err.Error())
			return err
		}
	}

	// Clean up temp directory
	os.RemoveAll(chunk.TempDir)

	// Set owner
	s.setOwner(finalPath)

	s.updateProgressCompleted(uploadID)
	return nil
}

// GetProgress returns progress for an upload
func (s *UploadService) GetProgress(uploadID string) (*models.Progress, bool) {
	return s.progressStore.Get(uploadID)
}

func (s *UploadService) updateProgressError(uploadID, errorMsg string) {
	if p, ok := s.progressStore.Get(uploadID); ok {
		p.Status = models.StatusFailed
		p.Error = errorMsg
		s.progressStore.Set(uploadID, p)
	}
}

func (s *UploadService) updateProgressCompleted(uploadID string) {
	if p, ok := s.progressStore.Get(uploadID); ok {
		p.Status = models.StatusCompleted
		p.Progress = 100
		p.UploadedBytes = p.TotalBytes
		s.progressStore.Set(uploadID, p)
	}
}
