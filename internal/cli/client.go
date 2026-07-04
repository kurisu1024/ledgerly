package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// httpClient returns the injected HTTP client, falling back to
// http.DefaultClient so command logic never nil-checks Deps.
func (d Deps) httpClient() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}
	return http.DefaultClient
}

// doAuthed sends req with the bearer token attached and returns the response
// body. A non-2xx status is an error carrying the status and the server's
// (trimmed) body text.
func doAuthed(deps Deps, req *http.Request, token string) ([]byte, error) {
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := deps.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s %s failed: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("server returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// postJSON POSTs body as JSON to url with bearer auth.
func postJSON(deps Deps, url, token string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return doAuthed(deps, req, token)
}

// getJSON GETs url with bearer auth.
func getJSON(deps Deps, url, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	return doAuthed(deps, req, token)
}
