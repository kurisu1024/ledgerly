package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kurisu1024/ledgerly/api/events"
	"github.com/spf13/cobra"
)

func newEventsCmd(deps Deps, conn *connection) *cobra.Command {
	events := &cobra.Command{
		Use:   "events",
		Short: "Manage audit events",
	}

	events.AddCommand(newEventsPostCmd(deps, conn))

	return events
}

func newEventsPostCmd(deps Deps, conn *connection) *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "post",
		Short: "POST one or more events to /v1/events",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEventsPost(cmd, deps, conn, file)
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "Read events JSON from this file instead of stdin")

	return cmd
}

// runEventsPost reads one event (or a JSON array of events) from --file or
// stdin, converts each to the api/events kebab-case wire DTO, and POSTs it
// to {serverURL}/v1/events with the resolved bearer token. Per event, a 202
// is reported as "accepted" — the write path is async, so this command must
// never claim the event was "persisted" or "stored". A non-2xx response
// (e.g. 503 when the server's queue is full) is reported on stderr and
// causes a non-zero exit. Missing token/server-url must fail before any
// HTTP call is attempted.
func runEventsPost(cmd *cobra.Command, deps Deps, conn *connection, file string) error {
	if conn.token() == "" {
		return fmt.Errorf("events post: no token configured (--token or %s)", EnvToken)
	}
	if conn.serverURL() == "" {
		return fmt.Errorf("events post: no server URL configured (--server-url or %s)", EnvServerURL)
	}

	var input io.Reader = cmd.InOrStdin()
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return fmt.Errorf("events post: failed to open --file: %w", err)
		}
		defer f.Close()
		input = f
	}

	evs, err := decodeEvents(input)
	if err != nil {
		return fmt.Errorf("events post: %w", err)
	}
	if len(evs) == 0 {
		return fmt.Errorf("events post: no events found in input")
	}

	cmd.SilenceUsage = true
	url := strings.TrimRight(conn.serverURL(), "/") + "/v1/events"
	for i, ev := range evs {
		body, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("events post: failed to encode event %d: %w", i, err)
		}
		if _, err := postJSON(deps, url, conn.token(), body); err != nil {
			return fmt.Errorf("events post: event %d: %w", i, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "accepted event %d (action %q) — queued for asynchronous write\n", i, ev.Action)
	}
	return nil
}

// decodeEvents reads either a JSON array of events, a single JSON object, or
// an NDJSON stream (one event object per line) into wire DTOs.
func decodeEvents(r io.Reader) ([]events.Event, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var evs []events.Event
		if err := json.Unmarshal(trimmed, &evs); err != nil {
			return nil, fmt.Errorf("failed to parse events array: %w", err)
		}
		return evs, nil
	}

	// A single object or NDJSON: decode objects back-to-back.
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	var evs []events.Event
	for dec.More() {
		var ev events.Event
		if err := dec.Decode(&ev); err != nil {
			return nil, fmt.Errorf("failed to parse event JSON: %w", err)
		}
		evs = append(evs, ev)
	}
	return evs, nil
}
