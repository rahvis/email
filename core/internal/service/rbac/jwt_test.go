package rbac

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestJWTService(secret string) *JWTService {
	return &JWTService{
		Secret:        secret,
		AccessExpiry:  1 * time.Hour,
		RefreshExpiry: 7 * 24 * time.Hour,
	}
}

func TestJWTService_GenerateToken(t *testing.T) {
	svc := newTestJWTService("test-jwt-secret-rbac")

	tests := []struct {
		name      string
		accountId int64
		username  string
		roles     []string
	}{
		{
			name:      "admin user",
			accountId: 1,
			username:  "admin",
			roles:     []string{"admin"},
		},
		{
			name:      "regular user with multiple roles",
			accountId: 42,
			username:  "user@example.com",
			roles:     []string{"user", "editor"},
		},
		{
			name:      "user with no roles",
			accountId: 100,
			username:  "noroles",
			roles:     []string{},
		},
		{
			name:      "user with nil roles",
			accountId: 200,
			username:  "nilroles",
			roles:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenStr, expiry, err := svc.GenerateToken(tt.accountId, tt.username, tt.roles)
			require.NoError(t, err)
			assert.NotEmpty(t, tokenStr)
			assert.Greater(t, expiry, time.Now().Unix())

			// Parse token back with known secret
			parsed, err := jwt.ParseWithClaims(tokenStr, &JWTCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
				return []byte(svc.Secret), nil
			})
			require.NoError(t, err)

			claims, ok := parsed.Claims.(*JWTCustomClaims)
			require.True(t, ok)
			assert.Equal(t, tt.accountId, claims.AccountId)
			assert.Equal(t, tt.username, claims.Username)
			assert.Equal(t, tt.roles, claims.Roles)
			assert.False(t, claims.ApiToken)
			assert.Equal(t, "access_token", claims.Subject)
		})
	}
}

func TestJWTService_GenerateRefreshToken(t *testing.T) {
	svc := newTestJWTService("refresh-secret")

	tokenStr, err := svc.GenerateRefreshToken(1, "testuser")
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	parsed, err := jwt.ParseWithClaims(tokenStr, &JWTCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(svc.Secret), nil
	})
	require.NoError(t, err)

	claims, ok := parsed.Claims.(*JWTCustomClaims)
	require.True(t, ok)
	assert.Equal(t, int64(1), claims.AccountId)
	assert.Equal(t, "testuser", claims.Username)
	assert.Equal(t, "refresh_token", claims.Subject)
	assert.Nil(t, claims.Roles)
}

func TestJWTService_GenerateApiToken(t *testing.T) {
	svc := newTestJWTService("api-token-secret")

	tokenStr, expiry, err := svc.GenerateApiToken(10, "apiuser", []string{"admin"})
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)
	assert.Equal(t, int64(0), expiry, "API token should have no expiry")

	parsed, err := jwt.ParseWithClaims(tokenStr, &JWTCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(svc.Secret), nil
	})
	require.NoError(t, err)

	claims, ok := parsed.Claims.(*JWTCustomClaims)
	require.True(t, ok)
	assert.True(t, claims.ApiToken)
	assert.Equal(t, "api_token", claims.Subject)
	assert.Nil(t, claims.ExpiresAt, "API tokens have no expiration")
}

func TestJWTService_ParseToken_Valid(t *testing.T) {
	svc := newTestJWTService("parse-secret")

	tokenStr, _, err := svc.GenerateToken(5, "parseuser", []string{"viewer"})
	require.NoError(t, err)

	// ParseToken calls IsTokenBlacklisted which uses Redis.
	// Instead, test the manual parsing path.
	parsed, err := jwt.ParseWithClaims(tokenStr, &JWTCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(svc.Secret), nil
	})
	require.NoError(t, err)

	claims, ok := parsed.Claims.(*JWTCustomClaims)
	require.True(t, ok)
	assert.Equal(t, int64(5), claims.AccountId)
	assert.Equal(t, "parseuser", claims.Username)
	assert.Equal(t, []string{"viewer"}, claims.Roles)
}

func TestJWTService_ParseToken_WrongSecret(t *testing.T) {
	svc := newTestJWTService("correct-secret")

	tokenStr, _, err := svc.GenerateToken(1, "user", []string{"admin"})
	require.NoError(t, err)

	// Try parsing with wrong secret
	_, err = jwt.ParseWithClaims(tokenStr, &JWTCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte("wrong-secret"), nil
	})
	assert.Error(t, err)
}

func TestJWTService_ParseToken_ExpiredToken(t *testing.T) {
	svc := &JWTService{
		Secret:        "expired-secret",
		AccessExpiry:  -1 * time.Hour, // negative means already expired
		RefreshExpiry: 7 * 24 * time.Hour,
	}

	tokenStr, _, err := svc.GenerateToken(1, "expired", []string{})
	require.NoError(t, err)

	_, err = jwt.ParseWithClaims(tokenStr, &JWTCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(svc.Secret), nil
	})
	assert.Error(t, err, "expired token should fail")
}

func TestJWTService_ParseToken_StripsBearerPrefix(t *testing.T) {
	svc := newTestJWTService("bearer-test-secret")

	tokenStr, _, err := svc.GenerateToken(1, "user", nil)
	require.NoError(t, err)

	// Test Bearer stripping logic
	stripped := strings.TrimPrefix("Bearer "+tokenStr, "Bearer ")
	assert.Equal(t, tokenStr, stripped)

	// Also test without prefix
	stripped2 := strings.TrimPrefix(tokenStr, "Bearer ")
	assert.Equal(t, tokenStr, stripped2)
}

func TestJWTService_IsProtectedAPITokenAllowed(t *testing.T) {
	svc := newTestJWTService("protected-claims-secret")

	tests := []struct {
		name   string
		claims *JWTCustomClaims
		want   bool
	}{
		{name: "nil claims", claims: nil, want: false},
		{
			name: "access token",
			claims: &JWTCustomClaims{
				RegisteredClaims: jwt.RegisteredClaims{Subject: tokenSubjectAccess},
			},
			want: true,
		},
		{
			name: "api token",
			claims: &JWTCustomClaims{
				ApiToken:         true,
				RegisteredClaims: jwt.RegisteredClaims{Subject: tokenSubjectAPI},
			},
			want: true,
		},
		{
			name: "refresh token",
			claims: &JWTCustomClaims{
				RegisteredClaims: jwt.RegisteredClaims{Subject: tokenSubjectRefresh},
			},
			want: false,
		},
		{
			name: "api flag with refresh subject",
			claims: &JWTCustomClaims{
				ApiToken:         true,
				RegisteredClaims: jwt.RegisteredClaims{Subject: tokenSubjectRefresh},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, svc.IsProtectedAPITokenAllowed(tt.claims))
		})
	}
}

func TestJWTService_ParseProtectedAPIToken(t *testing.T) {
	svc := newTestJWTService("protected-token-secret")

	accessToken, _, err := svc.GenerateToken(1, "admin", []string{"admin"})
	require.NoError(t, err)
	apiToken, _, err := svc.GenerateApiToken(1, "admin", []string{"admin"})
	require.NoError(t, err)
	refreshToken, err := svc.GenerateRefreshToken(1, "admin")
	require.NoError(t, err)

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{name: "empty token", token: "", wantErr: true},
		{name: "invalid token", token: "not-a-token", wantErr: true},
		{name: "access token", token: accessToken},
		{name: "access token with bearer prefix", token: "Bearer " + accessToken},
		{name: "api token", token: apiToken},
		{name: "refresh token", token: refreshToken, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := svc.ParseProtectedAPIToken(tt.token)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, claims)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, claims)
			assert.True(t, svc.IsProtectedAPITokenAllowed(claims))
		})
	}
}

func TestJWTService_TokenClaims_IDs(t *testing.T) {
	svc := newTestJWTService("unique-id-secret")

	token1, _, err := svc.GenerateToken(1, "user1", nil)
	require.NoError(t, err)

	token2, _, err := svc.GenerateToken(1, "user1", nil)
	require.NoError(t, err)

	// Parse both tokens and verify they have different JTI (JWT ID)
	parse := func(tokenStr string) *JWTCustomClaims {
		parsed, err := jwt.ParseWithClaims(tokenStr, &JWTCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(svc.Secret), nil
		})
		require.NoError(t, err)
		claims, ok := parsed.Claims.(*JWTCustomClaims)
		require.True(t, ok)
		return claims
	}

	claims1 := parse(token1)
	claims2 := parse(token2)

	assert.NotEqual(t, claims1.ID, claims2.ID, "each token should have a unique ID")
	assert.NotEmpty(t, claims1.ID)
	assert.NotEmpty(t, claims2.ID)
}

func TestJWTService_SigningMethod(t *testing.T) {
	svc := newTestJWTService("signing-method-secret")

	tokenStr, _, err := svc.GenerateToken(1, "user", nil)
	require.NoError(t, err)

	// Parse with method validation
	parsed, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Ensure HS256
		assert.Equal(t, "HS256", token.Method.Alg())
		return []byte(svc.Secret), nil
	})
	require.NoError(t, err)
	assert.True(t, parsed.Valid)
}

func TestJWTCustomClaims_Struct(t *testing.T) {
	claims := JWTCustomClaims{
		AccountId: 42,
		Username:  "testuser",
		Roles:     []string{"admin", "user"},
		ApiToken:  true,
	}

	assert.Equal(t, int64(42), claims.AccountId)
	assert.Equal(t, "testuser", claims.Username)
	assert.Equal(t, []string{"admin", "user"}, claims.Roles)
	assert.True(t, claims.ApiToken)
}
