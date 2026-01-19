package handlers

import (
	"bufio"
	"encoding/json"
	"filemanager-api/internal/middleware"
	"filemanager-api/internal/models"
	"filemanager-api/internal/services"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// UploadHandler handles upload-related HTTP requests
type UploadHandler struct {
	progressStore *models.ProgressStore
}

// NewUploadHandler creates a new upload handler
func NewUploadHandler(progressStore *models.ProgressStore) *UploadHandler {
	return &UploadHandler{progressStore: progressStore}
}

// getUploadService returns an upload service for the current user
func (h *UploadHandler) getUploadService(c *fiber.Ctx) *services.UploadService {
	userCtx := middleware.GetUserContext(c)
	if userCtx == nil {
		return nil
	}
	return services.NewUploadService(userCtx.BasePath, userCtx.UserSite, h.progressStore)
}

// Upload handles POST /api/v1/upload with streaming for large files
func (h *UploadHandler) Upload(c *fiber.Ctx) error {
	svc := h.getUploadService(c)
	if svc == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(
			models.NewErrorResponse("Unauthorized", "AUTH_ERROR", "User context not found"),
		)
	}

	contentType := c.Get("Content-Type")
	if contentType == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "CONTENT_TYPE_REQUIRED", "Content-Type header is required"),
		)
	}

	// Parse boundary from Content-Type
	boundary, err := parseBoundary(contentType)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_CONTENT_TYPE", err.Error()),
		)
	}

	// Get multipart form data without loading entire file into memory
	// Use the raw request body stream for large file handling
	// If the body is small, fasthttp might buffer it and RequestBodyStream() returns nil
	var reader *multipart.Reader
	bodyStream := c.Context().RequestBodyStream()
	if bodyStream != nil {
		reader = multipart.NewReader(bodyStream, boundary)
	} else {
		reader = multipart.NewReader(bytes.NewReader(c.Body()), boundary)
	}

	// Get destination from form data
	destination := ""

	var filePart *multipart.Part
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(
				models.NewErrorResponse("Bad Request", "FORM_PARSE_ERROR", err.Error()),
			)
		}

		if part.FormName() == "file" {
			filePart = part
			break
		}

		if part.FormName() == "destination" {
			destBytes, _ := io.ReadAll(part)
			destination = string(destBytes)
		}
	}

	if filePart == nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "FILE_REQUIRED", "File is required"),
		)
	}

	filename := filePart.FileName()
	if filename == "" {
		filename = "uploaded_file"
	}

	// Upload using streaming - the reader will stream data as it's received
	uploadID, err := svc.Upload(filename, destination, filePart, int64(c.Request().Header.ContentLength()))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			models.NewErrorResponse("Failed to upload file", "UPLOAD_ERROR", err.Error()),
		)
	}

	progress, _ := svc.GetProgress(uploadID)

	return c.Status(fiber.StatusAccepted).JSON(models.NewSuccessResponse("Upload started", fiber.Map{
		"upload_id": uploadID,
		"progress":  progress,
	}))
}

// parseBoundary extracts the boundary parameter from Content-Type header
func parseBoundary(contentType string) (string, error) {
	for _, part := range strings.Split(contentType, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "boundary=") {
			boundary := strings.TrimPrefix(part, "boundary=")
			if boundary[0] == '"' && boundary[len(boundary)-1] == '"' {
				boundary = boundary[1 : len(boundary)-1]
			}
			return boundary, nil
		}
	}
	return "", fmt.Errorf("boundary not found in Content-Type")
}

// ChunkedUpload handles POST /api/v1/upload/chunked
func (h *UploadHandler) ChunkedUpload(c *fiber.Ctx) error {
	svc := h.getUploadService(c)
	if svc == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(
			models.NewErrorResponse("Unauthorized", "AUTH_ERROR", "User context not found"),
		)
	}

	// Check if this is init or chunk upload
	action := c.FormValue("action", "upload")

	if action == "init" {
		// Initialize chunked upload
		filename := c.FormValue("filename")
		destination := c.FormValue("destination", "")
		totalSize, _ := strconv.ParseInt(c.FormValue("total_size", "0"), 10, 64)
		chunkSize, _ := strconv.Atoi(c.FormValue("chunk_size", "65536"))

		if filename == "" || totalSize == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(
				models.NewErrorResponse("Bad Request", "INVALID_PARAMS", "Filename and total_size are required"),
			)
		}

		chunk, err := svc.InitChunkedUpload(filename, destination, totalSize, chunkSize)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(
				models.NewErrorResponse("Failed to init chunked upload", "INIT_ERROR", err.Error()),
			)
		}

		return c.JSON(models.NewSuccessResponse("Chunked upload initialized", fiber.Map{
			"upload_id":    chunk.ID,
			"total_chunks": chunk.TotalChunks,
			"chunk_size":   chunk.ChunkSize,
		}))
	}

	// Upload chunk
	uploadID := c.FormValue("upload_id")
	chunkIndex, _ := strconv.Atoi(c.FormValue("chunk_index", "0"))

	if uploadID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_UPLOAD_ID", "Upload ID is required"),
		)
	}

	file, err := c.FormFile("chunk")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "CHUNK_REQUIRED", "Chunk data is required"),
		)
	}

	src, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			models.NewErrorResponse("Failed to open chunk", "CHUNK_OPEN_ERROR", err.Error()),
		)
	}
	defer src.Close()

	data := make([]byte, file.Size)
	if _, err := src.Read(data); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			models.NewErrorResponse("Failed to read chunk", "CHUNK_READ_ERROR", err.Error()),
		)
	}

	if err := svc.UploadChunk(uploadID, chunkIndex, data); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			models.NewErrorResponse("Failed to upload chunk", "CHUNK_UPLOAD_ERROR", err.Error()),
		)
	}

	progress, _ := svc.GetProgress(uploadID)

	return c.JSON(models.NewSuccessResponse("Chunk uploaded", fiber.Map{
		"upload_id": uploadID,
		"progress":  progress,
	}))
}

// Progress handles GET /api/v1/upload/progress/:id (SSE)
func (h *UploadHandler) Progress(c *fiber.Ctx) error {
	uploadID := c.Params("id")
	if uploadID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_ID", "Upload ID is required"),
		)
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				progress, ok := h.progressStore.Get(uploadID)
				if !ok {
					fmt.Fprintf(w, "data: {\"error\": \"upload not found\"}\n\n")
					w.Flush()
					return
				}

				data, _ := json.Marshal(progress)
				fmt.Fprintf(w, "data: %s\n\n", data)
				w.Flush()

				if progress.Status == models.StatusCompleted || progress.Status == models.StatusFailed {
					return
				}
			}
		}
	})

	return nil
}

// WebSocketProgress handles WS /api/v1/upload/ws/:id
func (h *UploadHandler) WebSocketProgress(c *websocket.Conn) {
	uploadID := c.Params("id")
	if uploadID == "" {
		c.WriteJSON(fiber.Map{"error": "Upload ID is required"})
		c.Close()
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			progress, ok := h.progressStore.Get(uploadID)
			if !ok {
				c.WriteJSON(fiber.Map{"error": "upload not found"})
				c.Close()
				return
			}

			if err := c.WriteJSON(progress); err != nil {
				return
			}

			if progress.Status == models.StatusCompleted || progress.Status == models.StatusFailed {
				c.Close()
				return
			}
		}
	}
}
