package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrPathTraversal   = errors.New("path traversal detected")
	ErrOutsideBasePath = errors.New("path is outside allowed base path")
	ErrInvalidPath     = errors.New("invalid path")
)

// SanitizePath cleans and validates a path
func SanitizePath(path string) string {
	// Clean the path
	cleaned := filepath.Clean(path)
	
	// Remove any leading slashes for relative paths
	cleaned = strings.TrimPrefix(cleaned, "/")
	
	return cleaned
}

// ValidatePath ensures the path is safe and within the base path
func ValidatePath(basePath, requestedPath string) (string, error) {
	// Clean and join the paths
	cleanBase := filepath.Clean(basePath)
	cleanReq := SanitizePath(requestedPath)
	
	// If empty path, return base path
	if cleanReq == "" || cleanReq == "." {
		return cleanBase, nil
	}
	
	// Join base path with requested path
	fullPath := filepath.Join(cleanBase, cleanReq)
	
	// Resolve any symlinks and get absolute path
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidPath, err)
	}
	
	absBase, err := filepath.Abs(cleanBase)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidPath, err)
	}
	
	// Check for path traversal - ensure the path is under base path
	if !strings.HasPrefix(absPath, absBase) {
		return "", ErrPathTraversal
	}
	
	return absPath, nil
}

// GetRelativePath returns the path relative to the base path
func GetRelativePath(basePath, fullPath string) (string, error) {
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return "", err
	}
	
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return "", err
	}
	
	// Check if path escapes base
	if strings.HasPrefix(rel, "..") {
		return "", ErrOutsideBasePath
	}
	
	return rel, nil
}

// PathExists checks if a path exists
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir checks if path is a directory
func IsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// IsFile checks if path is a file
func IsFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// GenerateUniqueName generates a unique filename if file exists
func GenerateUniqueName(path string) string {
	if !PathExists(path) {
		return path
	}
	
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext)
	
	counter := 1
	for {
		newName := fmt.Sprintf("%s_%d%s", name, counter, ext)
		newPath := filepath.Join(dir, newName)
		if !PathExists(newPath) {
			return newPath
		}
		counter++
	}
}
