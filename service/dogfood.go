package service

import (
	"context"
	"crypto/rsa"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	ledgerly "github.com/kurisu1024/ledgerly/sdk/go"
	"go.uber.org/zap"
)

// Dogfood wiring (issue #27): service/ logs through the ledgerly Go SDK
// (issue #26, ADR-0001) into a dedicated internal ledgerly-self tenant, so a
// rule change, an auth failure, and an admin action all become demo-able,
// verifiable entries in our own audit log — verified end-to-end via
// `ledgerly verify` (CLI, #16) and the verify endpoint (#23).
//
// Only service/ migrates its log calls to slog for this. api/http and
// internal/audit/worker.go keep plain zap deliberately: that's breaker #1
// against recursive amplification — ingest-path logging never reaches the
// SDK, so a broken self-token or a stuck worker flush can't feed an
// unbounded stream of self-events back into the self-audit surface (see
// issue #27's approved plan, "Recursion/amplification" section).
//
// RED-stage note: env consts, selfTenantID, and selfFallbackRules below are
// real, deterministic values — they're data, not behavior, and both the RED
// tests and the eventual GREEN implementation depend on them being pinned
// now. mintSelfToken and newDogfood are stubs; GREEN replaces their bodies:
//
//   - mintSelfToken: an unsigned RS256-shaped dev JWT, tenant_id=selfTenantID,
//     sub=selfSubject — the same unsigned-token technique
//     internal/cli/token.go uses for `make load-events`. That helper
//     (runToken) is cobra/Deps-specific and unexported, so it isn't reusable
//     as-is; this mirrors its approach rather than duplicating its command
//     wiring.
//   - newDogfood: bridge service/'s zap core to slog via
//     zapslog.NewHandler(base.Core()) (go.uber.org/zap/exp/zapslog), and —
//     when cfg.enabled — wrap that bridge in
//     ledgerly.NewHandler(selfFallbackRules, bridge, WithBufferDir(...),
//     WithEventsURL(...), WithRulesURL(...), WithAsyncFirstFetch(), ...)
//     per the plan's bootstrap/shutdown decisions.
const (
	// EnvDogfood enables the dogfood wiring when set to a truthy value. Off
	// by default: zero SDK construction, slog migration inert (service/
	// logs through plain zap, matching pre-#27 behavior exactly).
	EnvDogfood = "LEDGERLY_DOGFOOD"

	// EnvDogfoodBufferDir names the SDK's disk buffer + sequence-state
	// directory for the dogfood Handler (ledgerly.WithBufferDir). Required
	// when dogfood is enabled.
	EnvDogfoodBufferDir = "LEDGERLY_DOGFOOD_BUFFER_DIR"

	// EnvDogfoodToken supplies a pre-signed self-token for verified-auth
	// mode (a JWTPublicKey configured on the server). Dev/unverified mode
	// self-mints a token via mintSelfToken instead and does not require
	// this env var; verified mode without it is a hard startup error
	// (errDogfoodTokenRequired) rather than a silent fallback to an
	// unsigned token a verified server would reject anyway.
	EnvDogfoodToken = "LEDGERLY_DOGFOOD_TOKEN"
)

// selfTenantName is the fixed input to the ledgerly-self tenant UUID
// derivation below — never read from config, so dogfood events never share
// a chain with demo/real tenants and every ledgerly deployment agrees on
// the same self tenant.
const selfTenantName = "ledgerly-self"

// selfSubject is the JWT sub claim on every self-minted dogfood token, and
// the actor/resource identity on service-lifecycle events.
const selfSubject = "ledgerly-self"

// selfTokenTTL is the lifetime of a self-minted dev token. Dev/unverified
// mode performs no expiry validation at all (decodeUnverifiedJWT skips
// every claim check), so one mint per dogfood construction suffices — no
// refresh loop needed.
const selfTokenTTL = time.Hour

// selfTenantID is the fixed, deterministic ledgerly-self tenant UUID
// (uuid.NewSHA1 over a fixed namespace + name, per the approved plan).
var selfTenantID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(selfTenantName))

// selfFallbackRules is the compiled-in fallback rule-set for the dogfood
// Handler: Phase A's small, demo-able self-audit surface. sdk.started and
// sdk.rules-activated are emitted by the SDK unconditionally — they bypass
// rule matching entirely (see ledgerly.Handler.emitRegime) — so only the
// service/ lifecycle events need an explicit rule here.
var selfFallbackRules = []ledgerly.Rule{
	{SchemaVersion: ledgerly.SchemaVersion, EventType: "service.started"},
	{SchemaVersion: ledgerly.SchemaVersion, EventType: "service.stopping"},
}

// errDogfoodNotImplemented is returned by every newDogfood and
// mintSelfToken call at this RED stage. GREEN replaces both function
// bodies and this sentinel goes away.
var errDogfoodNotImplemented = errors.New("service: dogfood wiring not implemented yet (issue #27)")

// errDogfoodTokenRequired is the sentinel newDogfood must return when
// dogfood is enabled, a JWTPublicKey is configured (verified-auth
// posture), and no EnvDogfoodToken was supplied.
var errDogfoodTokenRequired = errors.New("service: " + EnvDogfoodToken + " is required when dogfood is enabled under verified JWT auth")

// dogfoodConfig groups newDogfood's inputs, decoupled from the process
// environment so it's directly constructible in tests without env
// mutation. now defaults to time.Now when nil.
type dogfoodConfig struct {
	enabled      bool
	bufferDir    string
	token        string // pre-signed self-token; required when jwtPublicKey != nil
	jwtPublicKey *rsa.PublicKey
	eventsURL    string
	rulesURL     string
	now          func() time.Time
}

// dogfood bundles the constructed self-audit slog.Logger — always present,
// even when disabled, per EnvDogfood's doc above — and its shutdown hook.
type dogfood struct {
	logger  *slog.Logger
	enabled bool

	sdk *ledgerly.Handler // nil when disabled
}

// Close stops the dogfood Handler's poller and drains in-flight delivery.
// A no-op when dogfood is disabled (or d is nil).
func (d *dogfood) Close(ctx context.Context) error {
	if d == nil || d.sdk == nil {
		return nil
	}
	return d.sdk.Close(ctx)
}

// mintSelfToken mints an unsigned dev JWT for the ledgerly-self tenant
// (tenant_id=selfTenantID, sub=selfSubject) — the same unsigned-token shape
// internal/cli/token.go crafts for `make load-events`. Only accepted by a
// server running with AllowUnverifiedJWT=true.
func mintSelfToken(now time.Time, ttl time.Duration) (string, error) {
	return "", errDogfoodNotImplemented
}

// newDogfood constructs the dogfood self-audit logging path: a slog.Logger
// bridged onto base's zap core, and — when cfg.enabled — teed through a
// ledgerly.Handler wired at the ledgerly-self tenant per ADR-0001 (see
// selfFallbackRules and the package doc above).
func newDogfood(base *zap.Logger, cfg dogfoodConfig) (*dogfood, error) {
	return nil, errDogfoodNotImplemented
}
