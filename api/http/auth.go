package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const tenantIDKey contextKey = "tenantID"

// jwtLeeway absorbs small clock skew between the token issuer and this server
// when validating exp/nbf.
const jwtLeeway = 30 * time.Second

// JWTClaims represents the claims we extract from the JWT token
type JWTClaims struct {
	TenantID string `json:"tenant_id"`
	jwt.RegisteredClaims
}

// authMiddleware extracts the tenant ID from the JWT token and adds it to the request context
func (t *T) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
			return
		}

		// Extract Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]

		claims, err := t.parseJWT(tokenString)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Validate tenant ID
		tenantID, err := uuid.Parse(claims.TenantID)
		if err != nil {
			http.Error(w, "Invalid tenant ID in token", http.StatusUnauthorized)
			return
		}

		// Add tenant ID to context
		ctx := context.WithValue(r.Context(), tenantIDKey, tenantID)
		next(w, r.WithContext(ctx))
	}
}

// parseJWT verifies and parses a JWT token.
//
// With a public key configured, the signature is verified (RS256 only —
// pinning the method defeats alg-confusion and alg=none attacks), exp is
// required, and exp/nbf are validated with jwtLeeway of skew. Without a key,
// tokens are accepted decode-only when AllowUnverifiedJWT was set (dev mode);
// otherwise every token is rejected.
func (t *T) parseJWT(tokenString string) (*JWTClaims, error) {
	if t.jwtSecret != nil {
		claims := &JWTClaims{}
		_, err := jwt.ParseWithClaims(tokenString, claims,
			func(token *jwt.Token) (any, error) { return t.jwtSecret, nil },
			jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
			jwt.WithExpirationRequired(),
			jwt.WithLeeway(jwtLeeway),
		)
		if err != nil {
			return nil, fmt.Errorf("verifying token: %w", err)
		}
		return claims, nil
	}

	if t.allowUnverifiedJWT {
		return decodeUnverifiedJWT(tokenString)
	}

	return nil, fmt.Errorf("no JWT public key configured and unverified tokens are not allowed")
}

// decodeUnverifiedJWT decodes a token's payload without verifying the
// signature. Dev mode only — reachable solely via Config.AllowUnverifiedJWT.
func decodeUnverifiedJWT(tokenString string) (*JWTClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	var claims JWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	return &claims, nil
}

// getTenantID extracts the tenant ID from the request context
func getTenantID(ctx context.Context) (uuid.UUID, error) {
	tenantID, ok := ctx.Value(tenantIDKey).(uuid.UUID)
	if !ok {
		return uuid.Nil, fmt.Errorf("tenant ID not found in context")
	}
	return tenantID, nil
}
