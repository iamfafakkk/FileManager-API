package utils

import (
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultBufferSize = 64 * 1024 // 64KB buffer for file operations
)

// CopyFile copies a file from src to dst with buffered I/O
func CopyFile(src, dst string, preserveMetadata bool) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	// Create destination directory if needed
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	// Use buffered copy
	buf := make([]byte, DefaultBufferSize)
	if _, err := io.CopyBuffer(dstFile, srcFile, buf); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Preserve metadata if requested
	if preserveMetadata {
		if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
			return fmt.Errorf("failed to set permissions: %w", err)
		}
		if err := os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
			return fmt.Errorf("failed to set timestamps: %w", err)
		}
	}

	return nil
}

// CopyFileWithProgress copies a file and reports progress
func CopyFileWithProgress(src, dst string, progressFn func(written, total int64)) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	totalSize := srcInfo.Size()
	var written int64
	buf := make([]byte, DefaultBufferSize)

	for {
		n, err := srcFile.Read(buf)
		if n > 0 {
			nw, werr := dstFile.Write(buf[:n])
			if werr != nil {
				return werr
			}
			written += int64(nw)
			if progressFn != nil {
				progressFn(written, totalSize)
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

// CopyDir copies a directory recursively
func CopyDir(src, dst string, preserveMetadata bool) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source directory: %w", err)
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath, preserveMetadata); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcPath, dstPath, preserveMetadata); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetMimeType returns the MIME type for a file
func GetMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		return "application/octet-stream"
	}
	return mimeType
}

// FormatFileSize formats bytes to human readable format
func FormatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// GetDirectorySize calculates total size of a directory
func GetDirectorySize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// FormatPermissions formats os.FileMode to string like "rwxr-xr-x"
func FormatPermissions(mode os.FileMode) string {
	var result strings.Builder
	
	for i := 0; i < 3; i++ {
		shift := uint(6 - i*3)
		if mode&(1<<(shift+2)) != 0 {
			result.WriteByte('r')
		} else {
			result.WriteByte('-')
		}
		if mode&(1<<(shift+1)) != 0 {
			result.WriteByte('w')
		} else {
			result.WriteByte('-')
		}
		if mode&(1<<shift) != 0 {
			result.WriteByte('x')
		} else {
			result.WriteByte('-')
		}
	}
	
	return result.String()
}
