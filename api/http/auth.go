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
	"go.uber.org/zap"
)

type contextKey string

const tenantIDKey contextKey = "tenantID"

// subjectKey stashes the JWT's sub claim in the request context — the
// actor for rule-mutation audit events (ADR-0002) and, more generally, any
// handler that needs to attribute an action to a caller rather than just a
// tenant.
//
// NOTE: in dev mode (AllowUnverifiedJWT, no signature check) the sub claim
// is entirely caller-controlled — flagged for security review, same caveat
// as the rest of decodeUnverifiedJWT.
const subjectKey contextKey = "subject"

// jwtLeeway absorbs small clock skew between the token issuer and this server
// when validating exp/nbf/iat.
const jwtLeeway = 30 * time.Second

// maxTokenLifetime caps exp-iat so a misconfigured or compromised issuer
// cannot mint effectively non-expiring credentials.
const maxTokenLifetime = 24 * time.Hour

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
			// Generic response to the caller; full detail server-side.
			t.logger.Warn("rejected JWT",
				zap.Error(err), zap.String("remote_addr", r.RemoteAddr))
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Validate tenant ID
		tenantID, err := uuid.Parse(claims.TenantID)
		if err != nil {
			t.logger.Warn("rejected JWT: invalid tenant ID claim",
				zap.Error(err), zap.String("remote_addr", r.RemoteAddr))
			http.Error(w, "Invalid tenant ID in token", http.StatusUnauthorized)
			return
		}

		// Add tenant ID (and, when present, the caller's subject) to context
		ctx := context.WithValue(r.Context(), tenantIDKey, tenantID)
		if claims.Subject != "" {
			ctx = context.WithValue(ctx, subjectKey, claims.Subject)
		}
		next(w, r.WithContext(ctx))
	}
}

// parseJWT verifies and parses a JWT token.
//
// With a public key configured, the signature is verified (RS256 only —
// pinning the method defeats alg-confusion and alg=none attacks), exp is
// required, iat (when present) must not be in the future, exp-iat is capped
// at maxTokenLifetime, and time claims get jwtLeeway of skew. Without a key,
// tokens are accepted decode-only when AllowUnverifiedJWT was set (dev mode)
// — note dev mode performs NO claim validation, including expiry; otherwise
// every token is rejected.
func (t *T) parseJWT(tokenString string) (*JWTClaims, error) {
	if t.jwtPublicKey != nil {
		claims := &JWTClaims{}
		_, err := jwt.ParseWithClaims(tokenString, claims,
			func(token *jwt.Token) (any, error) { return t.jwtPublicKey, nil },
			jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
			jwt.WithExpirationRequired(),
			jwt.WithIssuedAt(),
			jwt.WithLeeway(jwtLeeway),
		)
		if err != nil {
			return nil, fmt.Errorf("verifying token: %w", err)
		}
		if claims.IssuedAt != nil && claims.ExpiresAt != nil &&
			claims.ExpiresAt.Sub(claims.IssuedAt.Time) > maxTokenLifetime {
			return nil, fmt.Errorf("token lifetime exceeds %s", maxTokenLifetime)
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

// getSubject extracts the JWT sub claim stashed in the request context by
// authMiddleware. Tokens without a sub claim stash nothing, so this errors
// for them — callers decide whether an anonymous subject is acceptable.
func getSubject(ctx context.Context) (string, error) {
	subject, ok := ctx.Value(subjectKey).(string)
	if !ok {
		return "", fmt.Errorf("subject not found in context")
	}
	return subject, nil
}
