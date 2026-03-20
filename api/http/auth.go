package http

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type contextKey string

const tenantIDKey contextKey = "tenantID"

// JWTClaims represents the claims we extract from the JWT token
type JWTClaims struct {
	TenantID string `json:"tenant_id"`
	Sub      string `json:"sub"`
	Exp      int64  `json:"exp"`
	Iat      int64  `json:"iat"`
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

		// Parse JWT token (simple implementation without signature verification for now)
		claims, err := parseJWT(tokenString, t.jwtSecret)
		if err != nil {
			http.Error(w, "Invalid token: "+err.Error(), http.StatusUnauthorized)
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

// parseJWT parses a JWT token and extracts claims
// For simplicity, this does minimal validation - just decodes the payload
func parseJWT(tokenString string, publicKey *rsa.PublicKey) (*JWTClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	// Parse claims
	var claims JWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// TODO: Add signature verification using publicKey when needed
	// For now, we're keeping it simple as requested

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
