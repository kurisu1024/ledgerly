package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTempFile writes data to a temp file and returns its path, cleaned up
// automatically when the test ends.
func writeTempFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}
