package http_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	apihttp "github.com/kurisu1024/ledgerly/api/http"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
	"go.uber.org/zap"
)

func mustGenerateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// createUnsignedJWT builds a structurally valid token with a dummy signature,
// mirroring what `make load-events` sends in dev mode.
func createUnsignedJWT(tenantID uuid.UUID) string {
	headerJSON, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	payloadJSON, _ := json.Marshal(map[string]any{
		"tenant_id": tenantID.String(),
		"sub":       "test-user",
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	return fmt.Sprintf("%s.%s.%s",
		base64.RawURLEncoding.EncodeToString(headerJSON),
		base64.RawURLEncoding.EncodeToString(payloadJSON),
		base64.RawURLEncoding.EncodeToString([]byte("dummy-signature")))
}

func postEvent(t *testing.T, server *apihttp.T, token string) int {
	t.Helper()
	event := Event{
		OccurredAt: time.Now().UTC(),
		Action:     "project.create",
		Actor:      map[string]string{"id": "user1", "type": "user"},
		Resource:   map[string]string{"type": "project", "id": "p1"},
	}
	eventJSON, _ := json.Marshal(event)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(eventJSON))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	return w.Code
}

func TestJWTSignatureVerification(t *testing.T) {
	t.Log("\tGiven an HTTP server configured with an RSA public key")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := apihttp.New(ctx, memory.New(), testConfig(), zap.NewNop())
	defer server.Close()

	tenantID := uuid.New()
	validClaims := func() jwt.MapClaims {
		return jwt.MapClaims{
			"tenant_id": tenantID.String(),
			"sub":       "test-user",
			"iat":       time.Now().Unix(),
			"exp":       time.Now().Add(time.Hour).Unix(),
		}
	}

	tests := []struct {
		name  string
		token func(t *testing.T) string
		want  int
	}{
		{"valid RS256-signed token", func(t *testing.T) string {
			return signJWT(validClaims())
		}, http.StatusAccepted},

		{"signed with a different key", func(t *testing.T) string {
			otherKey := mustGenerateKey(t)
			s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims()).SignedString(otherKey)
			if err != nil {
				t.Fatal(err)
			}
			return s
		}, http.StatusUnauthorized},

		{"alg=none", func(t *testing.T) string {
			s, err := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims()).
				SignedString(jwt.UnsafeAllowNoneSignatureType)
			if err != nil {
				t.Fatal(err)
			}
			return s
		}, http.StatusUnauthorized},

		{"HS256 signed with the public key bytes (alg confusion)", func(t *testing.T) string {
			der, err := x509.MarshalPKIXPublicKey(&testKey.PublicKey)
			if err != nil {
				t.Fatal(err)
			}
			s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims()).SignedString(der)
			if err != nil {
				t.Fatal(err)
			}
			return s
		}, http.StatusUnauthorized},

		{"expired token", func(t *testing.T) string {
			claims := validClaims()
			claims["exp"] = time.Now().Add(-time.Hour).Unix()
			return signJWT(claims)
		}, http.StatusUnauthorized},

		{"missing exp", func(t *testing.T) string {
			claims := validClaims()
			delete(claims, "exp")
			return signJWT(claims)
		}, http.StatusUnauthorized},

		{"tampered payload after signing", func(t *testing.T) string {
			// Splice a forged payload onto a genuine signature.
			token := strings.Split(signJWT(validClaims()), ".")
			forged := strings.Split(signJWT(jwt.MapClaims{
				"tenant_id": uuid.New().String(),
				"exp":       time.Now().Add(time.Hour).Unix(),
			}), ".")
			return token[0] + "." + forged[1] + "." + token[2]
		}, http.StatusUnauthorized},

		{"garbage token", func(t *testing.T) string {
			return "not.a.jwt"
		}, http.StatusUnauthorized},

		{"iat in the future", func(t *testing.T) string {
			claims := validClaims()
			claims["iat"] = time.Now().Add(time.Hour).Unix()
			return signJWT(claims)
		}, http.StatusUnauthorized},

		{"lifetime beyond the 24h cap", func(t *testing.T) string {
			claims := validClaims()
			claims["exp"] = time.Now().Add(10 * 365 * 24 * time.Hour).Unix()
			return signJWT(claims)
		}, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := postEvent(t, server, tt.token(t)); got != tt.want {
				t.Fatalf("\t%s\tExpected status %d, got %d", fail, tt.want, got)
			}
			t.Logf("\t%s\t%s → %d", pass, tt.name, tt.want)
		})
	}
}

func TestJWTNoKeyConfigured(t *testing.T) {
	t.Log("\tGiven servers without a public key")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	unsigned := createUnsignedJWT(uuid.New())

	{
		t.Log("\tWhen AllowUnverifiedJWT is false (production default)")
		cfg := apihttp.DefaultConfig()
		server := apihttp.New(ctx, memory.New(), cfg, zap.NewNop())
		defer server.Close()

		if got := postEvent(t, server, unsigned); got != http.StatusUnauthorized {
			t.Fatalf("\t%s\tExpected 401 with no key and no dev flag, got %d", fail, got)
		}
		t.Logf("\t%s\tRejected token when verification is impossible", pass)
	}

	{
		t.Log("\tWhen AllowUnverifiedJWT is true (dev mode)")
		cfg := apihttp.DefaultConfig()
		cfg.AllowUnverifiedJWT = true
		server := apihttp.New(ctx, memory.New(), cfg, zap.NewNop())
		defer server.Close()

		if got := postEvent(t, server, unsigned); got != http.StatusAccepted {
			t.Fatalf("\t%s\tExpected 202 in dev mode, got %d", fail, got)
		}
		t.Logf("\t%s\tAccepted unsigned token in explicit dev mode", pass)
	}

	{
		t.Log("\tWhen a key is set, the dev flag must not bypass verification")
		cfg := testConfig()
		cfg.AllowUnverifiedJWT = true
		server := apihttp.New(ctx, memory.New(), cfg, zap.NewNop())
		defer server.Close()

		if got := postEvent(t, server, unsigned); got != http.StatusUnauthorized {
			t.Fatalf("\t%s\tExpected 401: key configured must always verify, got %d", fail, got)
		}
		t.Logf("\t%s\tKey configured → verification enforced despite dev flag", pass)
	}
}
