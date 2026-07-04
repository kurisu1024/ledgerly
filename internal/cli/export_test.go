package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurisu1024/ledgerly/api/events"
)

func execExport(t *testing.T, deps Deps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewRootCmd(deps)
	root.SetArgs(append([]string{"export"}, args...))

	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)

	err = root.Execute()
	return out.String(), errOut.String(), err
}

func TestExportBlockIDQueryParam(t *testing.T) {
	var gotQuery string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("blockID")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]events.Event{sampleEvent()})
	}))
	defer srv.Close()

	blockID := "22222222-2222-2222-2222-222222222222"
	_, _, err := execExport(t, Deps{HTTPClient: srv.Client()},
		"--server-url", srv.URL, "--token", "test-token", "--block-id", blockID)
	if err != nil {
		t.Fatalf("export returned error: %v", err)
	}

	if gotPath != "/v1/export" {
		t.Errorf("path = %q, want /v1/export", gotPath)
	}
	if gotQuery != blockID {
		t.Errorf("blockID query param = %q, want %q", gotQuery, blockID)
	}
}

func TestExportJSONRoundTripsBase64Hashes(t *testing.T) {
	ev := sampleEvent()
	ev.PrevHash = []byte{0xDE, 0xAD, 0xBE, 0xEF}
	ev.EventHash = []byte{0xCA, 0xFE, 0xBA, 0xBE}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]events.Event{ev})
	}))
	defer srv.Close()

	stdout, _, err := execExport(t, Deps{HTTPClient: srv.Client()},
		"--server-url", srv.URL, "--token", "test-token", "-o", "json")
	if err != nil {
		t.Fatalf("export returned error: %v", err)
	}

	var got []events.Event
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("failed to unmarshal -o json output: %v\noutput: %s", err, stdout)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if !bytes.Equal(got[0].PrevHash, ev.PrevHash) {
		t.Errorf("PrevHash = %x, want %x", got[0].PrevHash, ev.PrevHash)
	}
	if !bytes.Equal(got[0].EventHash, ev.EventHash) {
		t.Errorf("EventHash = %x, want %x", got[0].EventHash, ev.EventHash)
	}
}

func TestExportTableOutputHasRowPerEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]events.Event{sampleEvent(), sampleEvent()})
	}))
	defer srv.Close()

	stdout, _, err := execExport(t, Deps{HTTPClient: srv.Client()},
		"--server-url", srv.URL, "--token", "test-token", "-o", "table")
	if err != nil {
		t.Fatalf("export returned error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	// Expect at least a header row plus one row per event.
	if len(lines) < 3 {
		t.Fatalf("expected a header row + 2 event rows, got %d lines: %q", len(lines), stdout)
	}
}

func TestExportWritesToOutFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]events.Event{sampleEvent()})
	}))
	defer srv.Close()

	outPath := filepath.Join(t.TempDir(), "export.json")
	stdout, _, err := execExport(t, Deps{HTTPClient: srv.Client()},
		"--server-url", srv.URL, "--token", "test-token", "-o", "json", "--out", outPath)
	if err != nil {
		t.Fatalf("export returned error: %v", err)
	}
	if stdout != "" {
		t.Errorf("expected no stdout when --out is set, got %q", stdout)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected --out file to be written: %v", err)
	}
	var got []events.Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to unmarshal --out file contents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event in --out file, got %d", len(got))
	}
}
