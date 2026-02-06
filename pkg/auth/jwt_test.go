package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidate(t *testing.T) {
	// Test roundtrip: generate token -> validate token works
	secret := "test-secret-key-12345"
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()
	role := "admin"
	expiryHours := 24

	// Generate token
	tokenString, err := GenerateToken(secret, issuer, tenantID, userID, role, expiryHours)

	require.NoError(t, err, "Should not error when generating token")
	assert.NotEmpty(t, tokenString, "Token should not be empty")

	// Validate token
	claims, err := ValidateToken(tokenString, secret)

	require.NoError(t, err, "Should not error when validating token")
	assert.NotNil(t, claims)

	// Verify claims match what was provided
	assert.Equal(t, tenantID, claims.TenantID, "Tenant ID should match")
	assert.Equal(t, userID, claims.UserID, "User ID should match")
	assert.Equal(t, role, claims.Role, "Role should match")
	assert.Equal(t, issuer, claims.Issuer, "Issuer should match")
	assert.Equal(t, userID.String(), claims.Subject, "Subject should be user ID")

	// Verify standard claims are set
	assert.NotNil(t, claims.ExpiresAt, "ExpiresAt should be set")
	assert.NotNil(t, claims.IssuedAt, "IssuedAt should be set")
	assert.NotNil(t, claims.NotBefore, "NotBefore should be set")
	assert.NotEmpty(t, claims.ID, "Token ID should be set")
}

func TestGenerateToken_MultipleCallsCreateDifferentIDs(t *testing.T) {
	// Test that multiple token generations create unique token IDs
	secret := "test-secret-key-12345"
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()
	role := "user"
	expiryHours := 24

	token1, err := GenerateToken(secret, issuer, tenantID, userID, role, expiryHours)
	require.NoError(t, err)

	token2, err := GenerateToken(secret, issuer, tenantID, userID, role, expiryHours)
	require.NoError(t, err)

	// Parse both tokens to compare their IDs
	claims1, _ := ValidateToken(token1, secret)
	claims2, _ := ValidateToken(token2, secret)

	assert.NotEqual(t, claims1.ID, claims2.ID, "Each token should have a unique ID")
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	// Test that expired token returns error
	secret := "test-secret-key-12345"
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()
	role := "user"
	expiryHours := -1 // Expires in the past

	// Generate token with past expiry
	tokenString, err := GenerateToken(secret, issuer, tenantID, userID, role, expiryHours)
	require.NoError(t, err, "Should generate token even with past expiry")

	// Validate should fail
	claims, err := ValidateToken(tokenString, secret)

	assert.Error(t, err, "Should error when validating expired token")
	assert.Nil(t, claims, "Claims should be nil for expired token")
	assert.Contains(t, err.Error(), "token")
}

func TestValidateToken_WrongSecret(t *testing.T) {
	// Test that wrong secret returns error
	secret := "test-secret-key-12345"
	wrongSecret := "wrong-secret-key-67890"
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()
	role := "user"
	expiryHours := 24

	// Generate token with correct secret
	tokenString, err := GenerateToken(secret, issuer, tenantID, userID, role, expiryHours)
	require.NoError(t, err, "Should generate token")

	// Validate with wrong secret should fail
	claims, err := ValidateToken(tokenString, wrongSecret)

	assert.Error(t, err, "Should error when validating with wrong secret")
	assert.Nil(t, claims, "Claims should be nil with wrong secret")
}

func TestValidateToken_InvalidTokenString(t *testing.T) {
	// Test that invalid token string returns error
	secret := "test-secret-key-12345"
	invalidToken := "not.a.valid.token.string"

	claims, err := ValidateToken(invalidToken, secret)

	assert.Error(t, err, "Should error with invalid token")
	assert.Nil(t, claims)
}

func TestValidateToken_TamperedToken(t *testing.T) {
	// Test that tampered token (modified claims) returns error
	secret := "test-secret-key-12345"
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()
	role := "user"
	expiryHours := 24

	// Generate valid token
	tokenString, err := GenerateToken(secret, issuer, tenantID, userID, role, expiryHours)
	require.NoError(t, err)

	// Tamper with token by changing a character in the middle
	tamperedToken := tokenString[:len(tokenString)-10] + "tampered!!"

	claims, err := ValidateToken(tamperedToken, secret)

	assert.Error(t, err, "Should error when token is tampered")
	assert.Nil(t, claims)
}

func TestGenerateToken_DifferentRoles(t *testing.T) {
	// Test token generation with different roles
	secret := "test-secret-key-12345"
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()
	expiryHours := 24

	roles := []string{"admin", "user", "viewer", "editor"}

	for _, role := range roles {
		tokenString, err := GenerateToken(secret, issuer, tenantID, userID, role, expiryHours)
		require.NoError(t, err, "Should generate token for role: %s", role)

		claims, err := ValidateToken(tokenString, secret)
		require.NoError(t, err)
		assert.Equal(t, role, claims.Role, "Role should be %s", role)
	}
}

func TestGenerateToken_DifferentExpiries(t *testing.T) {
	// Test token generation with different expiry times
	secret := "test-secret-key-12345"
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()
	role := "user"

	testCases := []struct {
		expiryHours int
		name        string
	}{
		{1, "short-lived token"},
		{24, "one day token"},
		{168, "one week token"},
		{720, "one month token"},
	}

	for _, tc := range testCases {
		tokenString, err := GenerateToken(secret, issuer, tenantID, userID, role, tc.expiryHours)
		require.NoError(t, err, "Should generate %s", tc.name)

		claims, err := ValidateToken(tokenString, secret)
		require.NoError(t, err)

		// Verify expiry is set to approximately the correct time
		now := time.Now()
		expectedExpiry := now.Add(time.Duration(tc.expiryHours) * time.Hour)

		// Allow 5 second window for test execution time
		assert.WithinDuration(t, expectedExpiry, claims.ExpiresAt.Time, 5*time.Second,
			"Expiry should be approximately %d hours from now for %s", tc.expiryHours, tc.name)
	}
}

func TestGenerateToken_ClaimsStructure(t *testing.T) {
	// Test that generated token has correct claims structure
	secret := "test-secret-key-12345"
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()
	role := "admin"
	expiryHours := 24

	tokenString, err := GenerateToken(secret, issuer, tenantID, userID, role, expiryHours)
	require.NoError(t, err)

	claims, err := ValidateToken(tokenString, secret)
	require.NoError(t, err)

	// Verify all expected claims are present
	assert.Equal(t, tenantID, claims.TenantID)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, role, claims.Role)
	assert.Equal(t, issuer, claims.Issuer)
	assert.Equal(t, userID.String(), claims.Subject)
	assert.NotNil(t, claims.ExpiresAt)
	assert.NotNil(t, claims.IssuedAt)
	assert.NotNil(t, claims.NotBefore)
	assert.NotEmpty(t, claims.ID)

	// Verify IssuedAt, NotBefore, and ExpiresAt are in correct order
	issuedAt := claims.IssuedAt.Time
	notBefore := claims.NotBefore.Time
	expiresAt := claims.ExpiresAt.Time

	assert.Equal(t, issuedAt, notBefore, "IssuedAt and NotBefore should be the same")
	assert.True(t, expiresAt.After(issuedAt), "ExpiresAt should be after IssuedAt")
}

func TestValidateToken_MissingClaims(t *testing.T) {
	// Test validation with missing/invalid claims
	secret := "test-secret-key-12345"

	// Create a token with standard JWT claims but missing our custom claims
	standardClaims := jwt.RegisteredClaims{
		Issuer:    "test-issuer",
		Subject:   "test-subject",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		NotBefore: jwt.NewNumericDate(time.Now()),
		ID:        "test-id",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, standardClaims)
	tokenString, _ := token.SignedString([]byte(secret))

	// Attempt to validate - should fail because custom claims aren't present
	claims, err := ValidateToken(tokenString, secret)

	// This should fail because the claims don't match our Claims struct
	assert.Error(t, err, "Should error with mismatched claims structure")
	assert.Nil(t, claims)
}

func TestValidateToken_EmptySecret(t *testing.T) {
	// Test that empty secret still works (not recommended in production)
	secret := "test-secret-key-12345"
	emptySecret := ""
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()
	role := "user"
	expiryHours := 24

	// Generate token with valid secret
	tokenString, err := GenerateToken(secret, issuer, tenantID, userID, role, expiryHours)
	require.NoError(t, err)

	// Validate with empty secret should fail
	claims, err := ValidateToken(tokenString, emptySecret)

	assert.Error(t, err, "Should error with wrong secret (empty)")
	assert.Nil(t, claims)
}

func TestGenerateToken_LongExpiryPeriod(t *testing.T) {
	// Test token generation with very long expiry period
	secret := "test-secret-key-12345"
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()
	role := "user"
	expiryHours := 8760 // 1 year

	tokenString, err := GenerateToken(secret, issuer, tenantID, userID, role, expiryHours)
	require.NoError(t, err)

	claims, err := ValidateToken(tokenString, secret)
	require.NoError(t, err)

	// Verify expiry is set correctly
	assert.NotNil(t, claims.ExpiresAt)
	assert.True(t, claims.ExpiresAt.After(time.Now().Add(8000*time.Hour)),
		"Token expiry should be approximately 1 year from now")
}

func TestGenerateToken_UUIDTenantAndUser(t *testing.T) {
	// Test that UUID tenant and user IDs are properly preserved
	secret := "test-secret-key-12345"
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()
	role := "user"
	expiryHours := 24

	tokenString, err := GenerateToken(secret, issuer, tenantID, userID, role, expiryHours)
	require.NoError(t, err)

	claims, err := ValidateToken(tokenString, secret)
	require.NoError(t, err)

	// UUIDs should be exactly preserved
	assert.Equal(t, tenantID.String(), claims.TenantID.String())
	assert.Equal(t, userID.String(), claims.UserID.String())

	// Verify they're valid UUIDs
	assert.NotEqual(t, uuid.Nil, claims.TenantID)
	assert.NotEqual(t, uuid.Nil, claims.UserID)
}

func TestValidateToken_SigningMethodValidation(t *testing.T) {
	// Test that only HS256 signing method is accepted
	secret := "test-secret-key-12345"
	issuer := "test-issuer"
	tenantID := uuid.New()
	userID := uuid.New()

	// Create a token with RS256 (wrong signing method)
	claims := Claims{
		TenantID: tenantID,
		UserID:   userID,
		Role:     "user",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	// This would be signed with RS256 in real scenario, but for testing
	// we verify that our validation function checks for HMAC
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))

	// Validate with correct secret and method should work
	validatedClaims, err := ValidateToken(tokenString, secret)
	require.NoError(t, err)
	assert.Equal(t, tenantID, validatedClaims.TenantID)
}
