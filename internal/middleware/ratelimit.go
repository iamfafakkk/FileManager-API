package middleware

import (
	"filemanager-api/internal/config"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// RateLimit returns configured rate limiting middleware
func RateLimit() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        config.AppConfig.RateLimitReqs,
		Expiration: time.Duration(config.AppConfig.RateLimitWindow) * time.Second,
		KeyGenerator: func(c *fiber.Ctx) string {
			// Use API key + IP for rate limiting
			return c.Get("X-API-Key") + "-" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success": false,
				"message": "Rate limit exceeded",
				"error": fiber.Map{
					"code":    "RATE_LIMIT_EXCEEDED",
					"details": "Too many requests, please try again later",
				},
			})
		},
	})
}

// UploadRateLimit returns rate limiting for upload endpoints (more restrictive)
func UploadRateLimit() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        10, // 10 uploads per window
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.Get("X-API-Key") + "-upload-" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success": false,
				"message": "Upload rate limit exceeded",
				"error": fiber.Map{
					"code":    "UPLOAD_RATE_LIMIT_EXCEEDED",
					"details": "Too many upload requests, please try again later",
				},
			})
		},
	})
}
