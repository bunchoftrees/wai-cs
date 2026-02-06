package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all service configuration.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	JWT      JWTConfig
	Upload   UploadConfig
	Scoring  ScoringConfig
}

type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
	MaxConns int
}

type JWTConfig struct {
	Secret      string
	Issuer      string
	ExpiryHours int
}

type UploadConfig struct {
	MaxFileSize     int64    // bytes
	TempDir         string
	AllowedTypes    []string
	BatchInsertSize int
}

type ScoringConfig struct {
	MaxRetries    int
	RetryBaseWait time.Duration
	BatchSize     int
	WorkerCount   int
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", "8080"),
			ReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 60*time.Second),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "ssiq"),
			Password: getEnv("DB_PASSWORD", "ssiq_dev_password"),
			DBName:   getEnv("DB_NAME", "ssiq"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
			MaxConns: int(getIntEnv("DB_MAX_CONNS", 20)),
		},
		JWT: JWTConfig{
			Secret:      getEnv("JWT_SECRET", "dev-secret-change-in-production"),
			Issuer:      getEnv("JWT_ISSUER", "workforce-ai"),
			ExpiryHours: getIntEnv("JWT_EXPIRY_HOURS", 24),
		},
		Upload: UploadConfig{
			MaxFileSize:     int64(getIntEnv("UPLOAD_MAX_SIZE_MB", 100)) * 1024 * 1024,
			TempDir:         getEnv("UPLOAD_TEMP_DIR", "/tmp/ssiq-uploads"),
			AllowedTypes:    []string{"text/csv", "application/csv"},
			BatchInsertSize: getIntEnv("UPLOAD_BATCH_INSERT_SIZE", 1000),
		},
		Scoring: ScoringConfig{
			MaxRetries:    getIntEnv("SCORING_MAX_RETRIES", 3),
			RetryBaseWait: getDurationEnv("SCORING_RETRY_BASE_WAIT", 2*time.Second),
			BatchSize:     getIntEnv("SCORING_BATCH_SIZE", 1000),
			WorkerCount:   getIntEnv("SCORING_WORKER_COUNT", 4),
		},
	}
}

// DSN returns the Postgres connection string.
func (d *DatabaseConfig) DSN() string {
	return "postgres://" + d.User + ":" + d.Password +
		"@" + d.Host + ":" + d.Port +
		"/" + d.DBName + "?sslmode=" + d.SSLMode
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
