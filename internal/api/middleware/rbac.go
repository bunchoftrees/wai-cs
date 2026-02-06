package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireRole returns middleware that enforces role-based access control
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract role from context
		roleInterface, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "user role not found in context"})
			c.Abort()
			return
		}

		userRole, ok := roleInterface.(string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "invalid role format"})
			c.Abort()
			return
		}

		// Check if user role is in allowed roles
		authorized := false
		for _, allowedRole := range allowedRoles {
			if userRole == allowedRole {
				authorized = true
				break
			}
		}

		if !authorized {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}
