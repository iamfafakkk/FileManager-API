package models

import "sync"

// ProgressStatus represents the status of an operation
type ProgressStatus string

const (
	StatusPending    ProgressStatus = "pending"
	StatusUploading  ProgressStatus = "uploading"
	StatusProcessing ProgressStatus = "processing"
	StatusCompleted  ProgressStatus = "completed"
	StatusFailed     ProgressStatus = "failed"
)

// Progress represents progress of an operation
type Progress struct {
	ID            string         `json:"id"`
	Filename      string         `json:"filename,omitempty"`
	Progress      int            `json:"progress"`
	UploadedBytes int64          `json:"uploaded_bytes"`
	TotalBytes    int64          `json:"total_bytes"`
	Status        ProgressStatus `json:"status"`
	Error         string         `json:"error,omitempty"`
}

// ProgressStore stores progress information in memory
type ProgressStore struct {
	mu   sync.RWMutex
	data map[string]*Progress
}

// NewProgressStore creates a new progress store
func NewProgressStore() *ProgressStore {
	return &ProgressStore{
		data: make(map[string]*Progress),
	}
}

// Set stores progress for an operation
func (ps *ProgressStore) Set(id string, progress *Progress) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.data[id] = progress
}

// Get retrieves progress for an operation
func (ps *ProgressStore) Get(id string) (*Progress, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	p, ok := ps.data[id]
	return p, ok
}

// Delete removes progress for an operation
func (ps *ProgressStore) Delete(id string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.data, id)
}

// Update updates progress and calculates percentage
func (ps *ProgressStore) Update(id string, uploadedBytes int64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if p, ok := ps.data[id]; ok {
		p.UploadedBytes = uploadedBytes
		if p.TotalBytes > 0 {
			p.Progress = int((uploadedBytes * 100) / p.TotalBytes)
		}
	}
}

// CompressRequest represents a compression request
type CompressRequest struct {
	Paths            []string `json:"paths" validate:"required,min=1"`
	Output           string   `json:"output" validate:"required"`
	CompressionLevel int      `json:"compression_level"`
}

// ExtractRequest represents an extraction request
type ExtractRequest struct {
	Source      string `json:"source" validate:"required"`
	Destination string `json:"destination" validate:"required"`
}
