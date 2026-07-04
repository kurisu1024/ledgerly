package service

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	ledgerly "github.com/kurisu1024/ledgerly/sdk/go"
	"go.uber.org/zap"
)

// TestSelfTenantIDStable pins the ledgerly-self tenant UUID derivation: a
// fixed uuid.NewSHA1 recipe over a fixed namespace + name, never read from
// config, so every ledgerly deployment (and every process on this one)
// agrees on the same self tenant across restarts.
func TestSelfTenantIDStable(t *testing.T) {
	want := uuid.NewSHA1(uuid.NameSpaceOID, []byte(selfTenantName))
	if selfTenantID != want {
		t.Fatalf("selfTenantID = %s, want %s (recipe changed?)", selfTenantID, want)
	}
	if selfTenantID.Version() != 5 {
		t.Fatalf("selfTenantID version = %d, want 5 (uuid.NewSHA1 output)", selfTenantID.Version())
	}
	// Recomputing must be idempotent — the same recipe, called again,
	// yields the same tenant. A configurable or random derivation would
	// fail this.
	if again := uuid.NewSHA1(uuid.NameSpaceOID, []byte(selfTenantName)); again != selfTenantID {
		t.Fatalf("selfTenantID derivation is not stable across calls: %s != %s", again, selfTenantID)
	}
}

// TestSelfFallbackRulesCoverage pins Phase A's compiled-in fallback rule
// coverage: service.started and service.stopping, both on the schema
// version the SDK understands. sdk.started/sdk.rules-activated need no
// rule — the SDK records them unconditionally.
func TestSelfFallbackRulesCoverage(t *testing.T) {
	byEventType := make(map[string]ledgerly.Rule, len(selfFallbackRules))
	for _, r := range selfFallbackRules {
		byEventType[r.EventType] = r
	}

	for _, want := range []string{"service.started", "service.stopping"} {
		rule, ok := byEventType[want]
		if !ok {
			t.Fatalf("selfFallbackRules is missing a rule for event-type %q", want)
		}
		if rule.SchemaVersion != ledgerly.SchemaVersion {
			t.Fatalf("rule for %q has schema-version %d, want %d", want, rule.SchemaVersion, ledgerly.SchemaVersion)
		}
	}
}

// TestMintSelfToken_ThreePartJWTWithSelfClaims is the RED case for
// mintSelfToken: GREEN must mint a 3-part (unsigned) JWT whose payload
// carries tenant_id=selfTenantID and sub=ledgerly-self.
func TestMintSelfToken_ThreePartJWTWithSelfClaims(t *testing.T) {
	token, err := mintSelfToken(time.Now(), time.Hour)
	if err != nil {
		t.Fatalf("mintSelfToken: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("mintSelfToken produced %d dot-separated parts, want 3: %q", len(parts), token)
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decoding JWT payload: %v", err)
	}

	var claims struct {
		TenantID string `json:"tenant_id"`
		Subject  string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshaling JWT payload: %v", err)
	}

	if claims.TenantID != selfTenantID.String() {
		t.Fatalf("token tenant_id = %q, want %q", claims.TenantID, selfTenantID.String())
	}
	if claims.Subject != selfSubject {
		t.Fatalf("token sub = %q, want %q", claims.Subject, selfSubject)
	}
}

// TestNewDogfood_DisabledIsPlainLogger is the RED case for the off switch:
// LEDGERLY_DOGFOOD unset (cfg.enabled == false) must construct without
// error, expose a usable logger, and never touch disk for the SDK buffer —
// "zero SDK construction" per the package doc.
func TestNewDogfood_DisabledIsPlainLogger(t *testing.T) {
	base := zap.NewNop()
	bufferDir := filepath.Join(t.TempDir(), "dogfood-buffer") // deliberately not pre-created

	df, err := newDogfood(base, dogfoodConfig{
		enabled:   false,
		bufferDir: bufferDir,
	})
	if err != nil {
		t.Fatalf("disabled dogfood: unexpected error: %v", err)
	}
	if df == nil {
		t.Fatalf("disabled dogfood: got nil *dogfood")
	}
	if df.enabled {
		t.Fatalf("disabled dogfood: df.enabled = true, want false")
	}
	if df.logger == nil {
		t.Fatalf("disabled dogfood: df.logger is nil, want a plain slog.Logger")
	}
	if df.sdk != nil {
		t.Fatalf("disabled dogfood: df.sdk is non-nil, want zero SDK construction")
	}
	if _, statErr := os.Stat(bufferDir); !os.IsNotExist(statErr) {
		t.Fatalf("disabled dogfood: buffer dir %s exists (stat err: %v), want it never created", bufferDir, statErr)
	}
}

// TestNewDogfood_EnabledUnderVerifiedAuthRequiresToken is the RED case for
// the hard startup error: dogfood enabled, a JWTPublicKey configured
// (verified-auth posture), and no LEDGERLY_DOGFOOD_TOKEN supplied must fail
// construction outright rather than silently minting an unsigned token a
// verified server would reject anyway.
func TestNewDogfood_EnabledUnderVerifiedAuthRequiresToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating test RSA key: %v", err)
	}

	base := zap.NewNop()
	_, err = newDogfood(base, dogfoodConfig{
		enabled:      true,
		bufferDir:    t.TempDir(),
		jwtPublicKey: &key.PublicKey,
		eventsURL:    "https://127.0.0.1:0/v1/events",
		// token intentionally left empty.
	})
	if !errors.Is(err, errDogfoodTokenRequired) {
		t.Fatalf("newDogfood error = %v, want errors.Is(err, errDogfoodTokenRequired)", err)
	}
}
