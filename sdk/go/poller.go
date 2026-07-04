package ledgerly

import (
	"context"
	"sync"
	"time"
)

// Regime transition actions, emitted as chained audit events (ADR-0001
// amendments): sdk.started (regime=fallback) at construction, and
// sdk.rules-activated on the first successful GET /v1/rules fetch.
const (
	ActionSDKStarted        = "sdk.started"
	ActionSDKRulesActivated = "sdk.rules-activated"
)

// fetchTimeout bounds every steady-state GET /v1/rules so a hung rules
// endpoint can never wedge the polling loop. The synchronous first fetch
// during NewHandler gets the tighter firstFetchTimeout instead: it runs on
// the constructing caller's critical path, where a hung endpoint would
// otherwise stall application startup for the full steady-state budget.
const (
	fetchTimeout      = 5 * time.Second
	firstFetchTimeout = 2 * time.Second
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

	// async defers the first fetch to the polling goroutine instead of
	// running it synchronously inside start (WithAsyncFirstFetch);
	// firstFetchTimeout bounds that first fetch either way and is a field
	// only so tests can shrink it.
	async             bool
	firstFetchTimeout time.Duration

	// etag and activated are only touched by fetchOnce, which runs either
	// synchronously inside start before the polling goroutine exists, or
	// exclusively on that single goroutine (async mode) — the
	// goroutine-start happens-before edge orders the two in sync mode.
	etag      string
	activated bool

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// newPoller constructs a poller. It does not start polling until start is
// called.
func newPoller(client *apiClient, refreshInterval time.Duration, async bool, onSwap func(RuleList, regimeEvent)) *poller {
	return &poller{
		client:            client,
		refreshInterval:   refreshInterval,
		onSwap:            onSwap,
		async:             async,
		firstFetchTimeout: firstFetchTimeout,
	}
}

// start performs one first fetch — synchronously by default, so a caller
// observing the handler right after construction sees the server's rules
// already active, or on the polling goroutine in async mode
// (WithAsyncFirstFetch), keeping construction off the network — then polls
// on refreshInterval until ctx is done or stop is called. A handler with
// no rules endpoint configured never polls: it stays on its compiled-in
// fallback for life.
func (p *poller) start(ctx context.Context) {
	if p.client.rulesURL == "" {
		return
	}

	pctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	if !p.async {
		p.fetchOnce(pctx, p.firstFetchTimeout)
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if p.async {
			p.fetchOnce(pctx, p.firstFetchTimeout)
		}
		ticker := time.NewTicker(p.refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pctx.Done():
				return
			case <-ticker.C:
				p.fetchOnce(pctx, fetchTimeout)
			}
		}
	}()
}

// fetchOnce performs a single conditional fetch. Errors (including a
// refused schema version) keep the current rule set and ETag; a 304 is a
// pure no-op; a 200 swaps the active rules, tagging the first success as
// the fallback→server-rules regime transition.
func (p *poller) fetchOnce(ctx context.Context, timeout time.Duration) {
	fctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	list, newETag, notModified, err := p.client.fetchRules(fctx, p.etag)
	if err != nil || notModified {
		return
	}
	p.etag = newETag

	var ev regimeEvent
	if !p.activated {
		p.activated = true
		ev = regimeEvent{Action: ActionSDKRulesActivated, ETag: newETag}
	}
	p.onSwap(list, ev)
}

// stop halts polling started by start and waits for the polling goroutine
// to exit.
func (p *poller) stop() {
	if p.cancel == nil {
		return
	}
	p.cancel()
	p.wg.Wait()
}
