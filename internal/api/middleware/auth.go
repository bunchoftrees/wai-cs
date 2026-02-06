package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/workforce-ai/site-selection-iq/pkg/auth"
	"github.com/workforce-ai/site-selection-iq/internal/config"
)

// AuthMiddleware validates JWT tokens from the Authorization header
func AuthMiddleware(cfg *config.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract Bearer token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		// Parse Bearer token
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			c.Abort()
			return
		}

		token := strings.TrimPrefix(authHeader, bearerPrefix)

		// Validate token
		claims, err := auth.ValidateToken(token, cfg.Secret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// Set context values
		c.Set("tenant_id", claims.TenantID)
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)

		c.Next()
	}
}
