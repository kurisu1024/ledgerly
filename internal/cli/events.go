package cli

import (
	"fmt"

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
//
// TODO(GREEN): RED-stage stub — always errors, never reads input or makes
// an HTTP request.
func runEventsPost(cmd *cobra.Command, deps Deps, conn *connection, file string) error {
	if conn.token() == "" {
		return fmt.Errorf("events post: no token configured (--token or %s)", EnvToken)
	}
	return fmt.Errorf("events post: not implemented")
}
