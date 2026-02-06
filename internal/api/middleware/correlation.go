package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CorrelationMiddleware handles correlation ID tracking
func CorrelationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check for existing X-Correlation-ID header
		correlationID := c.GetHeader("X-Correlation-ID")
		if correlationID == "" {
			// Generate a new UUID if missing
			correlationID = uuid.New().String()
		}

		// Set on context
		c.Set("correlation_id", correlationID)

		// Set response header
		c.Header("X-Correlation-ID", correlationID)

		c.Next()
	}
}
