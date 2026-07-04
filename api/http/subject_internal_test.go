package http

// Internal (white-box) test package: authMiddleware and getSubject are
// unexported, and this is the one case that needs to reach them directly
// rather than through the server's public HTTP surface.

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TestAuthMiddleware_PropagatesSubject pins the ADR-0002 requirement that
// the JWT's sub claim is available to handlers (rule-mutation audit events
// need an actor). RED: authMiddleware does not stash subjectKey yet, so
// getSubject always errors until GREEN.
func TestAuthMiddleware_PropagatesSubject(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating test key: %v", err)
	}

	tt := &T{jwtPublicKey: &key.PublicKey, logger: zap.NewNop()}

	tenantID := uuid.New()
	wantSubject := "user-42"
	claims := jwt.MapClaims{
		"tenant_id": tenantID.String(),
		"sub":       wantSubject,
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(time.Hour).Unix(),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatalf("signing test token: %v", err)
	}

	var gotSubject string
	var subErr error
	handler := tt.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		gotSubject, subErr = getSubject(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected middleware to accept the token, got status %d: %s", w.Code, w.Body.String())
	}
	if subErr != nil {
		t.Fatalf("expected subject to be present in context, got error: %v", subErr)
	}
	if gotSubject != wantSubject {
		t.Fatalf("expected subject %q, got %q", wantSubject, gotSubject)
	}
}
