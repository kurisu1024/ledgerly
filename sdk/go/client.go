package ledgerly

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// authorize sets the bearer token on req when an API key is configured.
func (c *apiClient) authorize(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

// postEvent POSTs an already-encoded event body to eventsURL and returns
// the response status code.
func (c *apiClient) postEvent(ctx context.Context, body []byte) (statusCode int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.eventsURL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("ledgerly: building POST %s: %w", c.eventsURL, err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("ledgerly: POST %s: %w", c.eventsURL, err)
	}
	defer drainAndClose(resp.Body)
	return resp.StatusCode, nil
}

// fetchRules performs a conditional GET rulesURL with If-None-Match: etag.
// notModified is true on a 304 (etag still current); an envelope with a
// schema-version other than SchemaVersion is refused (err != nil), keeping
// whatever rule set the caller already has active.
func (c *apiClient) fetchRules(ctx context.Context, etag string) (list RuleList, newETag string, notModified bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.rulesURL, nil)
	if err != nil {
		return RuleList{}, "", false, fmt.Errorf("ledgerly: building GET %s: %w", c.rulesURL, err)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	c.authorize(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return RuleList{}, "", false, fmt.Errorf("ledgerly: GET %s: %w", c.rulesURL, err)
	}
	defer drainAndClose(resp.Body)

	switch resp.StatusCode {
	case http.StatusNotModified:
		return RuleList{}, etag, true, nil
	case http.StatusOK:
		// Decoded below.
	default:
		return RuleList{}, "", false, fmt.Errorf("ledgerly: GET %s: unexpected status %d", c.rulesURL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return RuleList{}, "", false, fmt.Errorf("ledgerly: decoding rules envelope: %w", err)
	}
	if list.SchemaVersion != SchemaVersion {
		return RuleList{}, "", false, fmt.Errorf("ledgerly: refusing rules envelope schema-version %d (want %d)", list.SchemaVersion, SchemaVersion)
	}
	return list, resp.Header.Get("ETag"), false, nil
}

// drainAndClose consumes and closes an HTTP response body so the
// underlying connection can be reused.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 1<<20))
	_ = body.Close()
}
