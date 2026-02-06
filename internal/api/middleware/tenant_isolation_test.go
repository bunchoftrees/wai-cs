package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/workforce-ai/site-selection-iq/pkg/auth"
)

// TestTenantIsolation_ContextCarriesTenantFromJWT proves that the tenant_id
// extracted from the JWT is what downstream handlers receive.  If a handler
// uses this value to scope DB queries, different tokens can never bleed data
// across tenants.
func TestTenantIsolation_ContextCarriesTenantFromJWT(t *testing.T) {
	cfg := testJWTConfig()

	tenantA := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000") // Acme
	tenantB := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000") // Globex

	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Handler echoes back the tenant_id it sees on the request context.
	r.GET("/echo-tenant",
		AuthMiddleware(cfg),
		func(c *gin.Context) {
			tid := c.MustGet("tenant_id").(uuid.UUID)
			c.JSON(200, gin.H{"tenant_id": tid.String()})
		},
	)

	// --- Request from Tenant A ---
	tokenA := generateTestToken(tenantA, uuid.New(), "admin")
	reqA := httptest.NewRequest("GET", "/echo-tenant", nil)
	reqA.Header.Set("Authorization", "Bearer "+tokenA)
	wA := httptest.NewRecorder()
	r.ServeHTTP(wA, reqA)

	require.Equal(t, 200, wA.Code)

	var bodyA map[string]string
	require.NoError(t, json.Unmarshal(wA.Body.Bytes(), &bodyA))
	assert.Equal(t, tenantA.String(), bodyA["tenant_id"],
		"Tenant A's token should produce tenant A's context")

	// --- Request from Tenant B ---
	tokenB := generateTestToken(tenantB, uuid.New(), "analyst")
	reqB := httptest.NewRequest("GET", "/echo-tenant", nil)
	reqB.Header.Set("Authorization", "Bearer "+tokenB)
	wB := httptest.NewRecorder()
	r.ServeHTTP(wB, reqB)

	require.Equal(t, 200, wB.Code)

	var bodyB map[string]string
	require.NoError(t, json.Unmarshal(wB.Body.Bytes(), &bodyB))
	assert.Equal(t, tenantB.String(), bodyB["tenant_id"],
		"Tenant B's token should produce tenant B's context")

	// --- Cross-check: they must differ ---
	assert.NotEqual(t, bodyA["tenant_id"], bodyB["tenant_id"],
		"Two different tenants must never resolve to the same tenant_id")
}

// TestTenantIsolation_CannotForgeViaClaims verifies that a token signed
// with a different secret (i.e., a forged token claiming to be tenant A)
// is rejected at the middleware layer before any handler executes.
func TestTenantIsolation_CannotForgeViaClaims(t *testing.T) {
	cfg := testJWTConfig()

	gin.SetMode(gin.TestMode)
	r := gin.New()

	handlerCalled := false
	r.GET("/protected",
		AuthMiddleware(cfg),
		func(c *gin.Context) {
			handlerCalled = true
			c.JSON(200, gin.H{"ok": true})
		},
	)

	// Attacker generates a token using their own secret, claiming to be tenant A
	forgedToken, err := auth.GenerateToken(
		"attacker-secret-not-the-real-one",
		testIssuer,
		uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		uuid.New(),
		"admin",
		24,
	)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+forgedToken)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code, "Forged token must be rejected")
	assert.False(t, handlerCalled, "Handler must not execute with a forged token")
}

// TestTenantIsolation_TenantATokenCannotAccessTenantBRoute simulates an
// endpoint that enforces tenant scoping.  A stub handler checks that the
// JWT tenant matches the resource's tenant; tenant A's token should get a
// 404 (or equivalent) when trying to access tenant B's resource.
func TestTenantIsolation_TenantATokenCannotAccessTenantBRoute(t *testing.T) {
	cfg := testJWTConfig()

	tenantA := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	tenantB := uuid.MustParse("660e8400-e29b-41d4-a716-446655440000")

	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Simulates a handler that loads a resource belonging to tenant B.
	// The handler checks that the caller's tenant matches the resource owner.
	// This mirrors the real pattern: repositories use WHERE tenant_id = $1.
	r.GET("/resource/:id",
		AuthMiddleware(cfg),
		func(c *gin.Context) {
			callerTenant := c.MustGet("tenant_id").(uuid.UUID)

			// Simulate DB lookup that returns resource owned by tenant B
			resourceOwner := tenantB

			if callerTenant != resourceOwner {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			c.JSON(200, gin.H{"data": "secret-stuff"})
		},
	)

	// Tenant A tries to access tenant B's resource
	tokenA := generateTestToken(tenantA, uuid.New(), "admin")
	req := httptest.NewRequest("GET", "/resource/some-id", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 404, w.Code,
		"Tenant A must not see tenant B's resources")

	// Tenant B accesses their own resource — should work
	tokenB := generateTestToken(tenantB, uuid.New(), "admin")
	reqB := httptest.NewRequest("GET", "/resource/some-id", nil)
	reqB.Header.Set("Authorization", "Bearer "+tokenB)
	wB := httptest.NewRecorder()

	r.ServeHTTP(wB, reqB)

	assert.Equal(t, 200, wB.Code,
		"Tenant B should see their own resource")
}

// TestTenantIsolation_ExpiredTokenBlocked confirms that an expired token
// for any tenant is rejected before the handler runs, preventing stale
// credentials from crossing tenant boundaries.
func TestTenantIsolation_ExpiredTokenBlocked(t *testing.T) {
	cfg := testJWTConfig()

	gin.SetMode(gin.TestMode)
	r := gin.New()

	handlerCalled := false
	r.GET("/protected",
		AuthMiddleware(cfg),
		func(c *gin.Context) {
			handlerCalled = true
			c.JSON(200, gin.H{"ok": true})
		},
	)

	// Generate expired token (negative expiry)
	expiredToken, err := auth.GenerateToken(
		testSecret, testIssuer,
		uuid.New(), uuid.New(), "admin",
		-1, // expired
	)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code, "Expired token must be rejected")
	assert.False(t, handlerCalled, "Handler must not execute with expired token")
}

// TestTenantIsolation_ViewerCannotMutate verifies that the RBAC layer
// prevents a viewer-role token from accessing mutation endpoints, even
// within their own tenant.
func TestTenantIsolation_ViewerCannotMutate(t *testing.T) {
	cfg := testJWTConfig()
	tenantA := uuid.New()

	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.POST("/upload",
		AuthMiddleware(cfg),
		RequireRole("admin", "analyst"),
		func(c *gin.Context) {
			c.JSON(201, gin.H{"ok": true})
		},
	)

	token := generateTestToken(tenantA, uuid.New(), "viewer")
	req := httptest.NewRequest("POST", "/upload", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, 403, w.Code,
		"Viewer role should be forbidden from mutation endpoints")
}
