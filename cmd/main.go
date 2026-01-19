package main

import (
	"filemanager-api/internal/config"
	"filemanager-api/internal/handlers"
	"filemanager-api/internal/middleware"
	"filemanager-api/internal/models"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Create progress store
	progressStore := models.NewProgressStore()

	// Create Fiber app
	app := fiber.New(fiber.Config{
		BodyLimit:             int(cfg.MaxUploadSize),
		StreamRequestBody:     true,
		DisableStartupMessage: false,
		AppName:               "FileManager API v1.0",
		ReadTimeout:           time.Second * time.Duration(cfg.ReadTimeout),
		WriteTimeout:          time.Second * time.Duration(cfg.WriteTimeout),
		IdleTimeout:           time.Second * time.Duration(cfg.IdleTimeout),
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))
	app.Use(middleware.CORS())

	// API routes
	api := app.Group("/api/v1")

	// Apply auth middleware to all API routes
	api.Use(middleware.Auth())
	api.Use(middleware.RateLimit())

	// Initialize handlers
	fmHandler := handlers.NewFileManagerHandler(progressStore)
	uploadHandler := handlers.NewUploadHandler(progressStore)
	compressHandler := handlers.NewCompressHandler(progressStore)
	extractHandler := handlers.NewExtractHandler(progressStore)

	// File System routes (combined files + folders)
	fs := api.Group("/fs")
	fs.Get("/", fmHandler.List)                // List directory
	fs.Get("/disk-usage", fmHandler.GetDiskUsage) // Get disk usage
	fs.Get("/info/*", fmHandler.GetInfo)       // Get file/folder info
	fs.Get("/download/*", fmHandler.Download)  // Download file
	fs.Post("/file", fmHandler.CreateFile)     // Create file
	fs.Put("/file/*", fmHandler.UpdateFile)    // Update file content
	fs.Post("/folder", fmHandler.CreateFolder) // Create folder
	fs.Put("/rename/*", fmHandler.Rename)      // Rename file/folder
	fs.Delete("/*", fmHandler.Delete)          // Delete file/folder
	fs.Post("/copy", fmHandler.Copy)           // Copy files/folders
	fs.Post("/move", fmHandler.Move)           // Move files/folders

	// Upload routes
	upload := api.Group("/upload")
	upload.Use(middleware.UploadRateLimit())
	upload.Post("/", uploadHandler.Upload)
	upload.Post("/chunked", uploadHandler.ChunkedUpload)
	upload.Get("/progress/:id", uploadHandler.Progress)

	// WebSocket for upload progress
	app.Get("/api/v1/upload/ws/:id", websocket.New(uploadHandler.WebSocketProgress))

	// Compression routes
	compress := api.Group("/compress")
	compress.Post("/", compressHandler.Compress)
	compress.Get("/progress/:id", compressHandler.Progress)

	// Extraction routes
	extract := api.Group("/extract")
	extract.Post("/", extractHandler.Extract)
	extract.Get("/progress/:id", extractHandler.Progress)

	// Raw command routes
	rawHandler := handlers.NewRawCommandHandler()
	api.Post("/raw", rawHandler.Execute)

	// Health check (no auth)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "healthy",
			"version": "1.0.0",
		})
	})

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("Gracefully shutting down...")
		_ = app.Shutdown()
	}()

	// Start server
	log.Printf("Starting FileManager API on port %s", cfg.Port)
	log.Printf("Base path: %s", cfg.BasePath)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
