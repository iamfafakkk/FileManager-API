package handlers

import (
	"errors"
	"io"
	"net/url"

	"filemanager-api/internal/middleware"
	"filemanager-api/internal/models"
	"filemanager-api/internal/services"
	"filemanager-api/internal/utils"

	"github.com/gofiber/fiber/v2"
)

// FileManagerHandler handles all file and folder HTTP requests
type FileManagerHandler struct {
	progressStore *models.ProgressStore
}

// NewFileManagerHandler creates a new file manager handler
func NewFileManagerHandler(progressStore *models.ProgressStore) *FileManagerHandler {
	return &FileManagerHandler{progressStore: progressStore}
}

// getService returns a file manager service for the current user (local or remote)
func (h *FileManagerHandler) getService(c *fiber.Ctx) (*services.FileManagerService, error) {
	userCtx := middleware.GetUserContext(c)
	if userCtx == nil {
		return nil, services.ErrPermissionDenied
	}

	if userCtx.IsRemote && userCtx.SSHConfig != nil {
		// Create remote SSH service
		sshConfig := &services.SSHConfig{
			Host:       userCtx.SSHConfig.Host,
			Port:       userCtx.SSHConfig.Port,
			Username:   userCtx.SSHConfig.Username,
			PrivateKey: userCtx.SSHConfig.PrivateKey,
		}
		return services.NewRemoteFileManagerService(userCtx.BasePath, sshConfig, userCtx.UserSite)
	}

	// Local service
	return services.NewFileManagerService(userCtx.BasePath, userCtx.UserSite), nil
}

// handleServiceError handles errors from getService with proper error messages
func (h *FileManagerHandler) handleServiceError(c *fiber.Ctx, err error) error {
	if errors.Is(err, services.ErrSSHConnection) {
		return c.Status(fiber.StatusBadGateway).JSON(
			models.NewErrorResponse("SSH Connection Failed", "SSH_ERROR", err.Error()),
		)
	}
	return c.Status(fiber.StatusUnauthorized).JSON(
		models.NewErrorResponse("Unauthorized", "AUTH_ERROR", err.Error()),
	)
}

// List handles GET /api/v1/fs - List all files and folders
func (h *FileManagerHandler) List(c *fiber.Ctx) error {
	svc, err := h.getService(c)
	if err != nil {
		return h.handleServiceError(c, err)
	}
	if svc.IsRemote() {
		defer svc.Close()
	}

	path := c.Query("path", "")

	items, err := svc.List(path)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Failed to list directory", "LIST_ERROR", err.Error()),
		)
	}

	return c.JSON(models.NewSuccessResponse("Directory listed successfully", items))
}

// GetDiskUsage handles GET /api/v1/fs/disk-usage
func (h *FileManagerHandler) GetDiskUsage(c *fiber.Ctx) error {
	svc, err := h.getService(c)
	if err != nil {
		return h.handleServiceError(c, err)
	}
	if svc.IsRemote() {
		defer svc.Close()
	}

	path := c.Query("path", "")

	size, err := svc.GetDiskUsage(path)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			models.NewErrorResponse("Failed to calculate disk usage", "DISK_USAGE_ERROR", err.Error()),
		)
	}

	return c.JSON(models.NewSuccessResponse("Disk usage calculated", fiber.Map{
		"path":       path,
		"size_bytes": size,
		"size_human": utils.FormatFileSize(size),
	}))
}

// GetInfo handles GET /api/v1/fs/info/*
func (h *FileManagerHandler) GetInfo(c *fiber.Ctx) error {
	svc, err := h.getService(c)
	if err != nil {
		return h.handleServiceError(c, err)
	}
	if svc.IsRemote() {
		defer svc.Close()
	}

	path, _ := url.PathUnescape(c.Params("*"))
	if path == "" {
		path = "."
	}

	info, err := svc.GetInfo(path)
	if err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, services.ErrNotFound) {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(
			models.NewErrorResponse("Failed to get info", "GET_INFO_ERROR", err.Error()),
		)
	}

	return c.JSON(models.NewSuccessResponse("Info retrieved", info))
}

// Download handles GET /api/v1/fs/download/*
func (h *FileManagerHandler) Download(c *fiber.Ctx) error {
	svc, err := h.getService(c)
	if err != nil {
		return h.handleServiceError(c, err)
	}

	path, _ := url.PathUnescape(c.Params("*"))
	if path == "" {
		if svc.IsRemote() {
			svc.Close()
		}
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_PATH", "Path is required"),
		)
	}

	// For remote files, use the streaming approach
	if svc.IsRemote() {
		reader, info, err := svc.GetContent(path)
		if err != nil {
			svc.Close()
			status := fiber.StatusInternalServerError
			if errors.Is(err, services.ErrNotFound) {
				status = fiber.StatusNotFound
			} else if errors.Is(err, services.ErrNotAFile) {
				status = fiber.StatusBadRequest
			}
			return c.Status(status).JSON(
				models.NewErrorResponse("Failed to download", "DOWNLOAD_ERROR", err.Error()),
			)
		}

		// Read all content before closing SSH connection
		data, readErr := io.ReadAll(reader)
		reader.Close()
		svc.Close()
		if readErr != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(
				models.NewErrorResponse("Failed to download", "DOWNLOAD_ERROR", readErr.Error()),
			)
		}

		c.Set("Content-Type", info.MimeType)
		c.Set("Content-Disposition", "attachment; filename=\""+info.Name+"\"")
		return c.Send(data)
	}

	// For local files, use SendFile which is more reliable
	fullPath, err := svc.GetFullPath(path)
	if err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, services.ErrNotFound) {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(
			models.NewErrorResponse("Failed to download", "DOWNLOAD_ERROR", err.Error()),
		)
	}

	info, err := svc.GetInfo(path)
	if err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, services.ErrNotFound) {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(
			models.NewErrorResponse("Failed to download", "DOWNLOAD_ERROR", err.Error()),
		)
	}

	if info.IsDir {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Failed to download", "DOWNLOAD_ERROR", "Cannot download a directory"),
		)
	}

	c.Set("Content-Disposition", "attachment; filename=\""+info.Name+"\"")
	return c.SendFile(fullPath, false)
}

// CreateFile handles POST /api/v1/fs/file
func (h *FileManagerHandler) CreateFile(c *fiber.Ctx) error {
	svc, err := h.getService(c)
	if err != nil {
		return h.handleServiceError(c, err)
	}
	if svc.IsRemote() {
		defer svc.Close()
	}

	var req models.CreateFileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_BODY", err.Error()),
		)
	}

	if req.Path == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_PATH", "Path is required"),
		)
	}

	info, err := svc.CreateFile(req.Path, req.Content)
	if err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, services.ErrAlreadyExists) {
			status = fiber.StatusConflict
		}
		return c.Status(status).JSON(
			models.NewErrorResponse("Failed to create file", "CREATE_ERROR", err.Error()),
		)
	}

	return c.Status(fiber.StatusCreated).JSON(models.NewSuccessResponse("File created", info))
}

// UpdateFile handles PUT /api/v1/fs/file/*
func (h *FileManagerHandler) UpdateFile(c *fiber.Ctx) error {
	svc, err := h.getService(c)
	if err != nil {
		return h.handleServiceError(c, err)
	}
	if svc.IsRemote() {
		defer svc.Close()
	}

	path, _ := url.PathUnescape(c.Params("*"))
	if path == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_PATH", "Path is required"),
		)
	}

	var req models.UpdateFileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_BODY", err.Error()),
		)
	}

	info, err := svc.UpdateFile(path, req.Content)
	if err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, services.ErrNotFound) {
			status = fiber.StatusNotFound
		} else if errors.Is(err, services.ErrNotAFile) {
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(
			models.NewErrorResponse("Failed to update file", "UPDATE_ERROR", err.Error()),
		)
	}

	return c.JSON(models.NewSuccessResponse("File updated", info))
}

// CreateFolder handles POST /api/v1/fs/folder
func (h *FileManagerHandler) CreateFolder(c *fiber.Ctx) error {
	svc, err := h.getService(c)
	if err != nil {
		return h.handleServiceError(c, err)
	}
	if svc.IsRemote() {
		defer svc.Close()
	}

	var req models.CreateFolderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_BODY", err.Error()),
		)
	}

	if req.Path == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_PATH", "Path is required"),
		)
	}

	info, err := svc.CreateFolder(req.Path)
	if err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, services.ErrAlreadyExists) {
			status = fiber.StatusConflict
		}
		return c.Status(status).JSON(
			models.NewErrorResponse("Failed to create folder", "CREATE_ERROR", err.Error()),
		)
	}

	return c.Status(fiber.StatusCreated).JSON(models.NewSuccessResponse("Folder created", info))
}

// Rename handles PUT /api/v1/fs/rename/*
func (h *FileManagerHandler) Rename(c *fiber.Ctx) error {
	svc, err := h.getService(c)
	if err != nil {
		return h.handleServiceError(c, err)
	}
	if svc.IsRemote() {
		defer svc.Close()
	}

	path, _ := url.PathUnescape(c.Params("*"))
	if path == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_PATH", "Path is required"),
		)
	}

	var req models.RenameRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_BODY", err.Error()),
		)
	}

	if req.NewName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_NAME", "New name is required"),
		)
	}

	info, err := svc.Rename(path, req.NewName)
	if err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, services.ErrNotFound) {
			status = fiber.StatusNotFound
		} else if errors.Is(err, services.ErrAlreadyExists) {
			status = fiber.StatusConflict
		}
		return c.Status(status).JSON(
			models.NewErrorResponse("Failed to rename", "RENAME_ERROR", err.Error()),
		)
	}

	return c.JSON(models.NewSuccessResponse("Renamed successfully", info))
}

// Delete handles DELETE /api/v1/fs/*
func (h *FileManagerHandler) Delete(c *fiber.Ctx) error {
	svc, err := h.getService(c)
	if err != nil {
		return h.handleServiceError(c, err)
	}
	if svc.IsRemote() {
		defer svc.Close()
	}

	path, _ := url.PathUnescape(c.Params("*"))
	if path == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_PATH", "Path is required"),
		)
	}

	recursive := c.Query("recursive", "false") == "true"

	if err := svc.Delete(path, recursive); err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, services.ErrNotFound) {
			status = fiber.StatusNotFound
		} else if errors.Is(err, services.ErrFolderNotEmpty) {
			status = fiber.StatusConflict
		}
		return c.Status(status).JSON(
			models.NewErrorResponse("Failed to delete", "DELETE_ERROR", err.Error()),
		)
	}

	return c.JSON(models.NewSuccessResponse("Deleted successfully", nil))
}

// Copy handles POST /api/v1/fs/copy
func (h *FileManagerHandler) Copy(c *fiber.Ctx) error {
	svc, err := h.getService(c)
	if err != nil {
		return h.handleServiceError(c, err)
	}
	if svc.IsRemote() {
		defer svc.Close()
	}

	var req models.CopyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_BODY", err.Error()),
		)
	}

	if len(req.Sources) == 0 || req.Destination == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_REQUEST", "Sources and destination are required"),
		)
	}

	copied, err := svc.Copy(req.Sources, req.Destination, req.Overwrite)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			models.NewErrorResponse("Failed to copy", "COPY_ERROR", err.Error()),
		)
	}

	return c.JSON(models.NewSuccessResponse("Copied successfully", copied))
}

// Move handles POST /api/v1/fs/move
func (h *FileManagerHandler) Move(c *fiber.Ctx) error {
	svc, err := h.getService(c)
	if err != nil {
		return h.handleServiceError(c, err)
	}
	if svc.IsRemote() {
		defer svc.Close()
	}

	var req models.MoveRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_BODY", err.Error()),
		)
	}

	if len(req.Sources) == 0 || req.Destination == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_REQUEST", "Sources and destination are required"),
		)
	}

	moved, err := svc.Move(req.Sources, req.Destination, req.Overwrite)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			models.NewErrorResponse("Failed to move", "MOVE_ERROR", err.Error()),
		)
	}

	return c.JSON(models.NewSuccessResponse("Moved successfully", moved))
}
