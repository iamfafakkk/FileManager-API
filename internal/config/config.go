package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port            string
	BasePath        string
	APIKey          string
	MaxUploadSize   int64
	ChunkSize       int
	RateLimitReqs   int
	RateLimitWindow int
	LogLevel        string
	ReadTimeout     int
	WriteTimeout    int
	IdleTimeout     int
}

var AppConfig *Config

func Load() *Config {
	AppConfig = &Config{
		Port:            getEnv("PORT", "4000"),
		BasePath:        getEnv("BASE_PATH", "/home"),
		APIKey:          getEnv("API_KEY", "filemanager-secret-key"),
		MaxUploadSize:   getEnvInt64("MAX_UPLOAD_SIZE", 10737418240), // 10GB default
		ChunkSize:       getEnvInt("CHUNK_SIZE", 65536),              // 64KB default
		RateLimitReqs:   getEnvInt("RATE_LIMIT_REQUESTS", 100),
		RateLimitWindow: getEnvInt("RATE_LIMIT_WINDOW", 60),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		ReadTimeout:     getEnvInt("READ_TIMEOUT", 7200),  // 2 hours default
		WriteTimeout:    getEnvInt("WRITE_TIMEOUT", 7200), // 2 hours default
		IdleTimeout:     getEnvInt("IDLE_TIMEOUT", 10800), // 3 hours default
	}
	return AppConfig
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}
