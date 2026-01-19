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

// ExtractHandler handles extraction-related HTTP requests
type ExtractHandler struct {
	progressStore *models.ProgressStore
}

// NewExtractHandler creates a new extract handler
func NewExtractHandler(progressStore *models.ProgressStore) *ExtractHandler {
	return &ExtractHandler{progressStore: progressStore}
}

// getExtractService returns an extract service for the current user
func (h *ExtractHandler) getExtractService(c *fiber.Ctx) *services.ExtractService {
	userCtx := middleware.GetUserContext(c)
	if userCtx == nil {
		return nil
	}
	return services.NewExtractService(userCtx.BasePath, userCtx.UserSite, h.progressStore)
}

// Extract handles POST /api/v1/extract
func (h *ExtractHandler) Extract(c *fiber.Ctx) error {
	svc := h.getExtractService(c)
	if svc == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(
			models.NewErrorResponse("Unauthorized", "AUTH_ERROR", "User context not found"),
		)
	}

	var req models.ExtractRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_BODY", err.Error()),
		)
	}

	if req.Source == "" || req.Destination == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_REQUEST", "Source and destination are required"),
		)
	}

	result, err := svc.Extract(req.Source, req.Destination)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			models.NewErrorResponse("Failed to extract", "EXTRACT_ERROR", err.Error()),
		)
	}

	// Parse result to get extract ID and destination path
	parts := strings.SplitN(result, ":", 2)
	extractID := parts[0]
	destPath := ""
	if len(parts) > 1 {
		destPath = parts[1]
	}

	progress, _ := svc.GetProgress(extractID)

	return c.Status(fiber.StatusAccepted).JSON(models.NewSuccessResponse("Extraction started", fiber.Map{
		"extract_id":  extractID,
		"destination": destPath,
		"progress":    progress,
	}))
}

// Progress handles GET /api/v1/extract/progress/:id (SSE)
func (h *ExtractHandler) Progress(c *fiber.Ctx) error {
	extractID := c.Params("id")
	if extractID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_ID", "Extract ID is required"),
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
				progress, ok := h.progressStore.Get(extractID)
				if !ok {
					fmt.Fprintf(w, "data: {\"error\": \"extraction not found\"}\n\n")
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
