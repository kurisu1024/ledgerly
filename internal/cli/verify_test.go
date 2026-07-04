package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"testing"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/api/events"
	"github.com/kurisu1024/ledgerly/internal/audit"
)

var verifyActor = map[string]string{"id": "user_1", "type": "user"}
var verifyResource = map[string]string{"type": "project", "id": "proj_1"}
var verifyMetadata = map[string]string{"reason": "test"}

// buildVerifyChainDTOs builds a real, correctly hash-chained EventChain via
// NewEventChain + AppendEvent (never hand-rolled hashes) and returns it as
// the kebab-case wire DTOs the server/CLI actually exchange.
func buildVerifyChainDTOs(n int, tenantID uuid.UUID) []events.Event {
	c := audit.NewEventChain(n)
	for i := 0; i < n; i++ {
		a := maps.Clone(verifyActor)
		a["id"] = fmt.Sprintf("user-%v", i)
		c = audit.AppendEvent(c, audit.NewEvent(tenantID, a, "project.create", verifyResource, verifyMetadata))
	}
	dtos := make([]events.Event, 0, n)
	for _, e := range c.Events {
		dtos = append(dtos, events.FromAuditEvent(e))
	}
	return dtos
}

func execVerifyCmd(t *testing.T, args []string, stdin []byte) (stdout, stderr string, exitCode int) {
	t.Helper()
	root := NewRootCmd(DefaultDeps())
	root.SetArgs(append([]string{"verify"}, args...))

	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	if stdin != nil {
		root.SetIn(bytes.NewReader(stdin))
	}

	exitCode = Run(root)
	return out.String(), errOut.String(), exitCode
}

func TestVerifyValidFileExitsZero(t *testing.T) {
	dtos := buildVerifyChainDTOs(4, uuid.New())
	payload, _ := json.Marshal(dtos)

	_, stderr, code := execVerifyCmd(t, []string{writeTempFile(t, payload)}, nil)
	if code != 0 {
		t.Fatalf("expected exit 0 for a valid chain, got %d (stderr: %s)", code, stderr)
	}
}

func TestVerifyInterleavedMultiChainGroupsCorrectly(t *testing.T) {
	a := buildVerifyChainDTOs(3, uuid.New())
	b := buildVerifyChainDTOs(3, uuid.New())

	// Interleave: a0 b0 a1 b1 a2 b2
	var interleaved []events.Event
	for i := 0; i < 3; i++ {
		interleaved = append(interleaved, a[i], b[i])
	}
	payload, _ := json.Marshal(interleaved)

	stdout, stderr, code := execVerifyCmd(t, []string{writeTempFile(t, payload), "-o", "json"}, nil)
	if code != 0 {
		t.Fatalf("expected exit 0 when both interleaved chains verify, got %d (stdout: %s, stderr: %s)", code, stdout, stderr)
	}

	var results []ChainVerifyResult
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		t.Fatalf("failed to unmarshal -o json output: %v\noutput: %s", err, stdout)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 chain results (grouped by chain-id), got %d", len(results))
	}
}

func TestVerifyTamperedExitsOneNamingDetails(t *testing.T) {
	dtos := buildVerifyChainDTOs(4, uuid.New())
	dtos[2].Action = "project.delete" // breaks the hash at index 2

	payload, _ := json.Marshal(dtos)

	stdout, stderr, code := execVerifyCmd(t, []string{writeTempFile(t, payload)}, nil)
	if code != 1 {
		t.Fatalf("expected exit 1 for a tampered chain, got %d", code)
	}

	combined := stdout + stderr
	if !bytes.Contains([]byte(combined), []byte(dtos[0].ChainID)) {
		t.Errorf("expected output to name the chain-id %q, got: %s", dtos[0].ChainID, combined)
	}
}

func TestVerifyReadsFromStdinDash(t *testing.T) {
	dtos := buildVerifyChainDTOs(2, uuid.New())
	payload, _ := json.Marshal(dtos)

	_, stderr, code := execVerifyCmd(t, []string{"-"}, payload)
	if code != 0 {
		t.Fatalf("expected exit 0 reading a valid chain from stdin, got %d (stderr: %s)", code, stderr)
	}
}

func TestVerifyEmptyArrayExitsOneUnverifiable(t *testing.T) {
	payload := []byte(`[]`)

	_, _, code := execVerifyCmd(t, []string{writeTempFile(t, payload)}, nil)
	if code != 1 {
		t.Fatalf("expected exit 1 for an empty events array, got %d", code)
	}
}

func TestVerifyMalformedJSONExitsTwo(t *testing.T) {
	payload := []byte(`{not valid json`)

	_, _, code := execVerifyCmd(t, []string{writeTempFile(t, payload)}, nil)
	if code != 2 {
		t.Fatalf("expected exit 2 for malformed JSON, got %d", code)
	}
}

func TestVerifyJSONOutputIsKebabCase(t *testing.T) {
	dtos := buildVerifyChainDTOs(2, uuid.New())
	payload, _ := json.Marshal(dtos)

	stdout, stderr, code := execVerifyCmd(t, []string{writeTempFile(t, payload), "-o", "json"}, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr)
	}

	var raw []map[string]any
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("failed to unmarshal -o json output: %v\noutput: %s", err, stdout)
	}
	if len(raw) != 1 {
		t.Fatalf("expected 1 chain result, got %d", len(raw))
	}
	for _, key := range []string{"chain-id", "status", "failed-index", "length"} {
		if _, ok := raw[0][key]; !ok {
			t.Errorf("expected kebab-case key %q in json output, got keys %v", key, keysOf(raw[0]))
		}
	}
	if _, ok := raw[0]["chain_id"]; ok {
		t.Errorf("json output must use kebab-case, not snake_case (found chain_id)")
	}
}
