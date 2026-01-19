package middleware

import (
	"filemanager-api/internal/config"
	"filemanager-api/internal/models"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// SSHConfig holds SSH connection details from headers
type SSHConfig struct {
	Host       string
	Port       string
	Username   string
	PrivateKey string
}

// UserContext holds the authenticated user information
type UserContext struct {
	UserSite  string
	BasePath  string
	SSHConfig *SSHConfig
	IsRemote  bool
}

// Auth middleware validates API key and extracts usersite/SSH from headers
func Auth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		apiKey := c.Get("X-API-Key")
		if apiKey == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(
				models.NewErrorResponse("Unauthorized", "AUTH_REQUIRED", "API key is required"),
			)
		}

		if apiKey != config.AppConfig.APIKey {
			return c.Status(fiber.StatusUnauthorized).JSON(
				models.NewErrorResponse("Unauthorized", "INVALID_API_KEY", "Invalid API key"),
			)
		}

		// Get usersite from header - required for all operations
		userSite := c.Get("X-User-Site")
		if userSite == "" {
			return c.Status(fiber.StatusBadRequest).JSON(
				models.NewErrorResponse("Bad Request", "USERSITE_REQUIRED", "X-User-Site header is required"),
			)
		}

		// Check for SSH headers for remote server access
		sshHost := c.Get("X-Ssh-Host")
		sshUsername := c.Get("X-Ssh-Username")
		sshPort := c.Get("X-Ssh-Port")
		sshKey := c.Get("X-Ssh-Key")

		userCtx := &UserContext{
			UserSite: userSite,
			BasePath: config.AppConfig.BasePath + "/" + userSite,
			IsRemote: false,
		}

		// If SSH headers are present, configure for remote access
		if sshHost != "" && sshKey != "" {
			if sshPort == "" {
				sshPort = "22"
			}
			if sshUsername == "" {
				sshUsername = "root"
			}

			// Convert escaped newlines to real newlines
			// Headers can't contain real newlines, so we accept:
			// 1. Literal \n as text (e.g., "-----BEGIN...\nb3BlbnNza...")
			// 2. URL-encoded newlines %0A
			normalizedKey := sshKey
			normalizedKey = strings.ReplaceAll(normalizedKey, "\\n", "\n")
			normalizedKey = strings.ReplaceAll(normalizedKey, "%0A", "\n")
			normalizedKey = strings.ReplaceAll(normalizedKey, "%0a", "\n")
			
			// Trim any extra whitespace
			normalizedKey = strings.TrimSpace(normalizedKey)

			userCtx.SSHConfig = &SSHConfig{
				Host:       sshHost,
				Port:       sshPort,
				Username:   sshUsername,
				PrivateKey: normalizedKey,
			}
			userCtx.IsRemote = true
		}

		c.Locals("user", userCtx)

		return c.Next()
	}
}

// GetUserContext retrieves user context from fiber context
func GetUserContext(c *fiber.Ctx) *UserContext {
	if user, ok := c.Locals("user").(*UserContext); ok {
		return user
	}
	return nil
}
