package handlers

import (
	"bufio"
	"encoding/json"
	"filemanager-api/internal/middleware"
	"filemanager-api/internal/models"
	"filemanager-api/internal/services"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// CompressHandler handles compression-related HTTP requests
type CompressHandler struct {
	progressStore *models.ProgressStore
}

// NewCompressHandler creates a new compress handler
func NewCompressHandler(progressStore *models.ProgressStore) *CompressHandler {
	return &CompressHandler{progressStore: progressStore}
}

// getCompressService returns a compress service for the current user
func (h *CompressHandler) getCompressService(c *fiber.Ctx) *services.CompressService {
	userCtx := middleware.GetUserContext(c)
	if userCtx == nil {
		return nil
	}
	return services.NewCompressService(userCtx.BasePath, userCtx.UserSite, h.progressStore)
}

// Compress handles POST /api/v1/compress
func (h *CompressHandler) Compress(c *fiber.Ctx) error {
	svc := h.getCompressService(c)
	if svc == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(
			models.NewErrorResponse("Unauthorized", "AUTH_ERROR", "User context not found"),
		)
	}

	var req models.CompressRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_BODY", err.Error()),
		)
	}

	if len(req.Paths) == 0 || req.Output == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_REQUEST", "Paths and output are required"),
		)
	}

	if req.CompressionLevel < 0 {
		req.CompressionLevel = 6 // Default compression level
	}

	result, err := svc.Compress(req.Paths, req.Output, req.CompressionLevel)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			models.NewErrorResponse("Failed to compress", "COMPRESS_ERROR", err.Error()),
		)
	}

	// Parse result to get compress ID and output path
	parts := strings.SplitN(result, ":", 2)
	compressID := parts[0]
	outputPath := ""
	if len(parts) > 1 {
		outputPath = parts[1]
	}

	progress, _ := svc.GetProgress(compressID)

	return c.Status(fiber.StatusAccepted).JSON(models.NewSuccessResponse("Compression started", fiber.Map{
		"compress_id": compressID,
		"output":      outputPath,
		"progress":    progress,
	}))
}

// Progress handles GET /api/v1/compress/progress/:id (SSE)
func (h *CompressHandler) Progress(c *fiber.Ctx) error {
	compressID := c.Params("id")
	if compressID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_ID", "Compress ID is required"),
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
				progress, ok := h.progressStore.Get(compressID)
				if !ok {
					fmt.Fprintf(w, "data: {\"error\": \"compression not found\"}\n\n")
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
