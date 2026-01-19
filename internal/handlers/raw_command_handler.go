package handlers

import (
	"filemanager-api/internal/middleware"
	"filemanager-api/internal/models"
	"filemanager-api/internal/services"

	"github.com/gofiber/fiber/v2"
)

// RawCommandHandler handles raw command execution requests
type RawCommandHandler struct{}

// NewRawCommandHandler creates a new raw command handler
func NewRawCommandHandler() *RawCommandHandler {
	return &RawCommandHandler{}
}

// getRawCommandService returns a raw command service for the current user
func (h *RawCommandHandler) getRawCommandService(c *fiber.Ctx) *services.RawCommandService {
	userCtx := middleware.GetUserContext(c)
	if userCtx == nil {
		return nil
	}
	return services.NewRawCommandService(userCtx.BasePath, userCtx.UserSite)
}

// Execute handles POST /api/v1/raw - Execute raw commands
func (h *RawCommandHandler) Execute(c *fiber.Ctx) error {
	svc := h.getRawCommandService(c)
	if svc == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(
			models.NewErrorResponse("Unauthorized", "AUTH_ERROR", "User context not found"),
		)
	}

	// Parse commands array from request body
	var commands []string
	if err := c.BodyParser(&commands); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "INVALID_BODY", "Expected JSON array of commands"),
		)
	}

	if len(commands) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewErrorResponse("Bad Request", "EMPTY_COMMANDS", "At least one command is required"),
		)
	}

	// Execute commands
	results, err := svc.ExecuteCommands(commands)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			models.NewErrorResponse("Failed to execute commands", "EXEC_ERROR", err.Error()),
		)
	}

	return c.JSON(models.NewSuccessResponse("Commands executed", fiber.Map{
		"base_path": svc.GetBasePath(),
		"results":   results,
	}))
}
