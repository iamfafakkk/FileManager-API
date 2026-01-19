package models

import (
	"os"
	"time"
)

// FileInfo represents file metadata
type FileInfo struct {
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Size        int64       `json:"size"`
	IsDir       bool        `json:"is_dir"`
	Mode        os.FileMode `json:"mode"`
	ModTime     time.Time   `json:"mod_time"`
	Extension   string      `json:"extension,omitempty"`
	MimeType    string      `json:"mime_type,omitempty"`
	Permissions string      `json:"permissions"`
}

// FolderInfo represents folder metadata with contents
type FolderInfo struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Size     int64       `json:"size"`
	ModTime  time.Time   `json:"mod_time"`
	Mode     os.FileMode `json:"mode"`
	Children []FileInfo  `json:"children,omitempty"`
	Count    int         `json:"count"`
}

// CreateFileRequest represents a file creation request
type CreateFileRequest struct {
	Path    string `json:"path" validate:"required"`
	Content string `json:"content"`
}

// UpdateFileRequest represents a file update request
type UpdateFileRequest struct {
	Content string `json:"content"`
}

// CreateFolderRequest represents a folder creation request
type CreateFolderRequest struct {
	Path string `json:"path" validate:"required"`
}

// RenameRequest represents a rename request
type RenameRequest struct {
	NewName string `json:"new_name" validate:"required"`
}

// CopyRequest represents a copy/move request
type CopyRequest struct {
	Sources     []string `json:"sources" validate:"required,min=1"`
	Destination string   `json:"destination" validate:"required"`
	Overwrite   bool     `json:"overwrite"`
}

// MoveRequest represents a move request
type MoveRequest struct {
	Sources     []string `json:"sources" validate:"required,min=1"`
	Destination string   `json:"destination" validate:"required"`
	Overwrite   bool     `json:"overwrite"`
}

// DeleteRequest represents a delete request with options
type DeleteRequest struct {
	Recursive bool `json:"recursive"`
}
