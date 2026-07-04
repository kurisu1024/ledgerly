package ledgerly

import (
	"context"
	"net/http"
)

// apiClient talks to the ledgerly server: POST /v1/events for delivery and
// GET /v1/rules for the poller. A 202 from POST /v1/events means accepted
// into the async ingest queue, never persisted — see api/http/handlers.go.
type apiClient struct {
	eventsURL string
	rulesURL  string
	apiKey    string
	http      *http.Client
}

// newAPIClient constructs an apiClient. A nil hc falls back to
// http.DefaultClient.
func newAPIClient(eventsURL, rulesURL, apiKey string, hc *http.Client) *apiClient {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &apiClient{eventsURL: eventsURL, rulesURL: rulesURL, apiKey: apiKey, http: hc}
}

// postEvent POSTs an already-encoded event body to eventsURL and returns
// the response status code.
//
// STUB: not implemented.
func (c *apiClient) postEvent(ctx context.Context, body []byte) (statusCode int, err error) {
	return 0, ErrNotImplemented
}

// fetchRules performs a conditional GET rulesURL with If-None-Match: etag.
// notModified is true on a 304 (etag still current); an envelope with a
// schema-version other than SchemaVersion is refused (err != nil), keeping
// whatever rule set the caller already has active.
//
// STUB: not implemented.
func (c *apiClient) fetchRules(ctx context.Context, etag string) (list RuleList, newETag string, notModified bool, err error) {
	return RuleList{}, "", false, ErrNotImplemented
}
