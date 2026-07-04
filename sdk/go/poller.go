package ledgerly

import (
	"context"
	"time"
)

// Regime transition actions, emitted as chained audit events (ADR-0001
// amendments): sdk.started (regime=fallback) at construction, and
// sdk.rules-activated on the first successful GET /v1/rules fetch.
const (
	ActionSDKStarted        = "sdk.started"
	ActionSDKRulesActivated = "sdk.rules-activated"
)

// regimeEvent describes a rules-poller state transition to be recorded in
// the chain by the handler.
type regimeEvent struct {
	Action           string
	FallbackDuration time.Duration
	ETag             string
}

// poller fetches GET /v1/rules on refreshInterval, ETag-cached, and
// atomically swaps the active rule set on change via onSwap. An envelope
// schema-version other than SchemaVersion is refused, keeping the current
// rule set (and current ETag) in place.
type poller struct {
	client          *apiClient
	refreshInterval time.Duration
	onSwap          func(RuleList, regimeEvent)
}

// newPoller constructs a poller. It does not start polling until start is
// called.
func newPoller(client *apiClient, refreshInterval time.Duration, onSwap func(RuleList, regimeEvent)) *poller {
	return &poller{client: client, refreshInterval: refreshInterval, onSwap: onSwap}
}

// start begins polling in the background until ctx is done.
//
// STUB: not implemented.
func (p *poller) start(ctx context.Context) {}

// stop halts polling started by start.
//
// STUB: not implemented.
func (p *poller) stop() {}
