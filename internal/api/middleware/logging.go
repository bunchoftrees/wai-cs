package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// StructuredLogging provides structured JSON logging for all requests
func StructuredLogging() gin.HandlerFunc {
	logger := slog.Default()
	return LoggingMiddleware(logger, "site-selection-iq")
}

// LoggingMiddleware provides structured JSON logging for all requests
func LoggingMiddleware(logger *slog.Logger, serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// Extract context values
		tenantID, _ := c.Get("tenant_id")
		correlationID, _ := c.Get("correlation_id")
		userID, _ := c.Get("user_id")

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(startTime)
		durationMs := duration.Milliseconds()

		// Determine outcome
		statusCode := c.Writer.Status()
		var outcome string
		var level slog.Level

		switch {
		case statusCode >= 200 && statusCode < 300:
			outcome = "success"
			level = slog.LevelInfo
		case statusCode >= 400 && statusCode < 500:
			outcome = "client_error"
			level = slog.LevelWarn
		case statusCode >= 500:
			outcome = "server_error"
			level = slog.LevelError
		default:
			outcome = "unknown"
			level = slog.LevelInfo
		}

		// Build attributes
		attrs := []slog.Attr{
			slog.String("service", serviceName),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status_code", statusCode),
			slog.Int64("duration_ms", durationMs),
			slog.String("outcome", outcome),
		}

		// Add optional context values
		if tenantID != nil {
			attrs = append(attrs, slog.Any("tenant_id", tenantID))
		}
		if correlationID != nil {
			attrs = append(attrs, slog.Any("correlation_id", correlationID))
		}
		if userID != nil {
			attrs = append(attrs, slog.Any("user_id", userID))
		}

		// Add timestamp
		attrs = append(attrs, slog.Int64("timestamp", startTime.UnixMilli()))

		// Log with appropriate level
		logger.LogAttrs(c.Request.Context(), level, "request processed", attrs...)
	}
}
