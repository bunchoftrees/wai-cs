package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/workforce-ai/site-selection-iq/internal/config"
	"github.com/workforce-ai/site-selection-iq/pkg/auth"
)

const testSecret = "test-secret-key-for-middleware-tests"
const testIssuer = "test-issuer"

func testJWTConfig() *config.JWTConfig {
	return &config.JWTConfig{
		Secret:      testSecret,
		Issuer:      testIssuer,
		ExpiryHours: 24,
	}
}

func setupRouter(jwtCfg *config.JWTConfig) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORSMiddleware())
	r.Use(CorrelationMiddleware())
	return r
}

func generateTestToken(tenantID, userID uuid.UUID, role string) string {
	token, _ := auth.GenerateToken(testSecret, testIssuer, tenantID, userID, role, 24)
	return token
}

// ---------------------------------------------------------------------------
// Auth middleware
// ---------------------------------------------------------------------------

func TestAuthMiddleware_ValidToken(t *testing.T) {
	cfg := testJWTConfig()
	r := setupRouter(cfg)

	tenantID := uuid.New()
	userID := uuid.New()

	var capturedTenantID uuid.UUID
	var capturedRole string

	r.GET("/test", AuthMiddleware(cfg), func(c *gin.Context) {
		capturedTenantID = c.MustGet("tenant_id").(uuid.UUID)
		capturedRole = c.MustGet("role").(string)
		c.JSON(200, gin.H{"ok": true})
	})

	token := generateTestToken(tenantID, userID, "admin")
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, tenantID, capturedTenantID, "tenant_id should be extracted from JWT")
	assert.Equal(t, "admin", capturedRole)
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	cfg := testJWTConfig()
	r := setupRouter(cfg)
	r.GET("/test", AuthMiddleware(cfg), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	cfg := testJWTConfig()
	r := setupRouter(cfg)
	r.GET("/test", AuthMiddleware(cfg), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer totally-bogus-token")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func TestAuthMiddleware_WrongSecret(t *testing.T) {
	cfg := testJWTConfig()
	r := setupRouter(cfg)
	r.GET("/test", AuthMiddleware(cfg), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Generate token with a different secret
	token, err := auth.GenerateToken("wrong-secret", testIssuer, uuid.New(), uuid.New(), "admin", 24)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func TestAuthMiddleware_MalformedAuthorizationHeader(t *testing.T) {
	cfg := testJWTConfig()
	r := setupRouter(cfg)
	r.GET("/test", AuthMiddleware(cfg), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// No "Bearer " prefix
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic abc123")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// ---------------------------------------------------------------------------
// RBAC middleware
// ---------------------------------------------------------------------------

func TestRequireRole_AllowedRole(t *testing.T) {
	cfg := testJWTConfig()
	r := setupRouter(cfg)

	r.GET("/admin-only",
		AuthMiddleware(cfg),
		RequireRole("admin"),
		func(c *gin.Context) {
			c.JSON(200, gin.H{"ok": true})
		},
	)

	token := generateTestToken(uuid.New(), uuid.New(), "admin")
	req := httptest.NewRequest("GET", "/admin-only", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

func TestRequireRole_DeniedRole(t *testing.T) {
	cfg := testJWTConfig()
	r := setupRouter(cfg)

	r.GET("/admin-only",
		AuthMiddleware(cfg),
		RequireRole("admin"),
		func(c *gin.Context) {
			c.JSON(200, gin.H{"ok": true})
		},
	)

	token := generateTestToken(uuid.New(), uuid.New(), "viewer")
	req := httptest.NewRequest("GET", "/admin-only", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 403, w.Code)
}

func TestRequireRole_MultipleAllowedRoles(t *testing.T) {
	cfg := testJWTConfig()
	r := setupRouter(cfg)

	r.GET("/data",
		AuthMiddleware(cfg),
		RequireRole("admin", "analyst"),
		func(c *gin.Context) {
			c.JSON(200, gin.H{"ok": true})
		},
	)

	tests := []struct {
		role       string
		wantCode   int
	}{
		{"admin", 200},
		{"analyst", 200},
		{"viewer", 403},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			token := generateTestToken(uuid.New(), uuid.New(), tt.role)
			req := httptest.NewRequest("GET", "/data", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code, "role=%s", tt.role)
		})
	}
}

// ---------------------------------------------------------------------------
// CORS middleware
// ---------------------------------------------------------------------------

func TestCORSMiddleware_SetsHeaders(t *testing.T) {
	r := setupRouter(testJWTConfig())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	assert.Contains(t, w.Header().Get("Access-Control-Expose-Headers"), "X-Correlation-ID")
}

func TestCORSMiddleware_PreflightOptions(t *testing.T) {
	r := setupRouter(testJWTConfig())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 204, w.Code, "OPTIONS preflight should return 204 No Content")
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

// ---------------------------------------------------------------------------
// Correlation middleware
// ---------------------------------------------------------------------------

func TestCorrelationMiddleware_GeneratesID(t *testing.T) {
	r := setupRouter(testJWTConfig())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	corrID := w.Header().Get("X-Correlation-ID")
	assert.NotEmpty(t, corrID, "should generate a correlation ID")

	// Should be a valid UUID
	_, err := uuid.Parse(corrID)
	assert.NoError(t, err, "correlation ID should be a valid UUID")
}

func TestCorrelationMiddleware_PreservesExistingID(t *testing.T) {
	r := setupRouter(testJWTConfig())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	customID := "my-custom-correlation-id"
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Correlation-ID", customID)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, customID, w.Header().Get("X-Correlation-ID"),
		"should preserve the client-supplied correlation ID")
}
