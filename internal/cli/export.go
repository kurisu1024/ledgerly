package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kurisu1024/ledgerly/api/events"
	"github.com/spf13/cobra"
)

func newExportCmd(deps Deps, conn *connection) *cobra.Command {
	var blockID string
	var output string
	var out string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export events from /v1/export",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd, deps, conn, blockID, output, out)
		},
	}

	cmd.Flags().StringVar(&blockID, "block-id", "", "Restrict export to a single block (sent as the blockID query param)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table|json")
	cmd.Flags().StringVar(&out, "out", "", "Write output to this file instead of stdout")

	return cmd
}

// runExport GETs {serverURL}/v1/export (optionally filtered by --block-id)
// with the resolved bearer token, then renders the api/events.Event DTOs
// either as a tabwriter table (one row per event) or as JSON (the same
// kebab-case DTO the server returns, base64 hashes round-tripping as-is) —
// to stdout, or to --out when set.
func runExport(cmd *cobra.Command, deps Deps, conn *connection, blockID, output, out string) error {
	if conn.token() == "" {
		return fmt.Errorf("export: no token configured (--token or %s)", EnvToken)
	}
	if conn.serverURL() == "" {
		return fmt.Errorf("export: no server URL configured (--server-url or %s)", EnvServerURL)
	}
	if output != "table" && output != "json" {
		return fmt.Errorf("export: unknown output format %q (want table or json)", output)
	}

	cmd.SilenceUsage = true

	exportURL := strings.TrimRight(conn.serverURL(), "/") + "/v1/export"
	if blockID != "" {
		exportURL += "?" + url.Values{"blockID": {blockID}}.Encode()
	}

	body, err := getJSON(deps, exportURL, conn.token())
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	var evs []events.Event
	if err := json.Unmarshal(body, &evs); err != nil {
		return fmt.Errorf("export: failed to parse server response: %w", err)
	}

	var rendered bytes.Buffer
	switch output {
	case "json":
		enc := json.NewEncoder(&rendered)
		enc.SetIndent("", "  ")
		if err := enc.Encode(evs); err != nil {
			return fmt.Errorf("export: failed to encode events: %w", err)
		}
	case "table":
		if err := renderEventsTable(&rendered, evs); err != nil {
			return fmt.Errorf("export: failed to render table: %w", err)
		}
	}

	if out != "" {
		if err := os.WriteFile(out, rendered.Bytes(), 0o600); err != nil {
			return fmt.Errorf("export: failed to write --out file: %w", err)
		}
		return nil
	}
	_, err = cmd.OutOrStdout().Write(rendered.Bytes())
	return err
}

// renderEventsTable writes a tabwriter table with a header row and one row
// per event.
func renderEventsTable(w io.Writer, evs []events.Event) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCHAIN-ID\tTENANT-ID\tOCCURRED-AT\tACTION")
	for _, ev := range evs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			ev.ID, ev.ChainID, ev.TenantID, ev.OccurredAt.Format(time.RFC3339), ev.Action)
	}
	return tw.Flush()
}
