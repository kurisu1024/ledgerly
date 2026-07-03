package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kurisu1024/ledgerly/api/events"
)

func execEvents(t *testing.T, deps Deps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewRootCmd(deps)
	root.SetArgs(append([]string{"events"}, args...))

	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)

	err = root.Execute()
	return out.String(), errOut.String(), err
}

func sampleEvent() events.Event {
	return events.Event{
		TenantID: "11111111-1111-1111-1111-111111111111",
		Action:   "project.create",
		Actor:    map[string]string{"id": "user-1", "type": "user"},
		Resource: map[string]string{"type": "project", "id": "proj-1"},
	}
}

func TestEventsPostSuccess(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Errorf("failed to unmarshal posted body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(sampleEvent())
	}))
	defer srv.Close()

	payload, _ := json.Marshal(sampleEvent())

	stdout, _, err := execEvents(t, Deps{HTTPClient: srv.Client()},
		"post", "--server-url", srv.URL, "--token", "test-token", "--file", writeTempFile(t, payload))
	if err != nil {
		t.Fatalf("events post returned error: %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-token")
	}
	if _, ok := gotBody["tenant-id"]; !ok {
		t.Errorf("expected kebab-case %q key in posted body, got keys %v", "tenant-id", keysOf(gotBody))
	}
	if _, ok := gotBody["tenant_id"]; ok {
		t.Errorf("posted body must not use snake_case tenant_id key")
	}

	lower := strings.ToLower(stdout)
	if !strings.Contains(lower, "accepted") {
		t.Errorf("expected stdout to mention 'accepted', got %q", stdout)
	}
	if strings.Contains(lower, "persisted") || strings.Contains(lower, "stored") {
		t.Errorf("stdout must not claim the event was persisted/stored (202 is async), got %q", stdout)
	}
}

func TestEventsPostFromStdin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(sampleEvent())
	}))
	defer srv.Close()

	payload, _ := json.Marshal(sampleEvent())

	root := NewRootCmd(Deps{HTTPClient: srv.Client()})
	root.SetArgs([]string{"events", "post", "--server-url", srv.URL, "--token", "test-token"})
	root.SetIn(bytes.NewReader(payload))
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)

	if err := root.Execute(); err != nil {
		t.Fatalf("events post from stdin returned error: %v", err)
	}
	if !strings.Contains(strings.ToLower(out.String()), "accepted") {
		t.Errorf("expected stdout to mention 'accepted', got %q", out.String())
	}
}

func TestEventsPostQueueFull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Event queue is full", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	payload, _ := json.Marshal(sampleEvent())

	_, stderr, err := execEvents(t, Deps{HTTPClient: srv.Client()},
		"post", "--server-url", srv.URL, "--token", "test-token", "--file", writeTempFile(t, payload))
	if err == nil {
		t.Fatalf("expected non-zero exit when server returns 503")
	}
	if stderr == "" && err.Error() == "" {
		t.Fatalf("expected an error message on 503")
	}
}

func TestEventsPostMissingTokenErrorsBeforeHTTP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	payload, _ := json.Marshal(sampleEvent())

	_, _, err := execEvents(t, Deps{HTTPClient: srv.Client()},
		"post", "--server-url", srv.URL, "--file", writeTempFile(t, payload))
	if err == nil {
		t.Fatalf("expected an error when no token is configured")
	}
	if called {
		t.Fatalf("expected no HTTP request to be made when token is missing")
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
