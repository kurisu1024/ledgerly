package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func execToken(t *testing.T, deps Deps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewRootCmd(deps)
	root.SetArgs(append([]string{"token"}, args...))

	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)

	err = root.Execute()
	return out.String(), errOut.String(), err
}

func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestTokenCommand(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	tenantID := uuid.New().String()

	stdout, _, err := execToken(t, Deps{Now: fixedNow(now)}, "--tenant-id", tenantID, "--ttl", "2h")
	if err != nil {
		t.Fatalf("token command returned error: %v", err)
	}

	if !strings.HasSuffix(stdout, "\n") {
		t.Fatalf("expected trailing newline on stdout, got %q", stdout)
	}
	token := strings.TrimSuffix(stdout, "\n")
	if strings.Contains(token, "\n") {
		t.Fatalf("expected exactly one line of output (bare token), got %q", stdout)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected a 3-part JWT, got %d parts: %q", len(parts), token)
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("failed to base64-decode header: %v", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("failed to unmarshal header: %v", err)
	}
	if header.Alg != "RS256" {
		t.Errorf("header.alg = %q, want RS256", header.Alg)
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("failed to base64-decode payload: %v", err)
	}
	var claims struct {
		TenantID string `json:"tenant_id"`
		Iat      int64  `json:"iat"`
		Exp      int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		t.Fatalf("failed to unmarshal claims: %v", err)
	}

	if claims.TenantID != tenantID {
		t.Errorf("claims.tenant_id = %q, want %q", claims.TenantID, tenantID)
	}
	if claims.Iat != now.Unix() {
		t.Errorf("claims.iat = %d, want %d (injected Now)", claims.Iat, now.Unix())
	}
	if got, want := claims.Exp-claims.Iat, int64(2*time.Hour/time.Second); got != want {
		t.Errorf("claims.exp - claims.iat = %d, want %d (--ttl 2h)", got, want)
	}
}

func TestTokenCommandRejectsExcessiveTTL(t *testing.T) {
	_, stderr, err := execToken(t, Deps{Now: fixedNow(time.Now())}, "--ttl", "25h")
	if err == nil {
		t.Fatalf("expected --ttl 25h to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()+stderr), "ttl") &&
		!strings.Contains(strings.ToLower(err.Error()+stderr), "24h") {
		t.Errorf("expected error to mention the ttl/24h limit, got err=%q stderr=%q", err, stderr)
	}
}
