package cli

import (
	"fmt"

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
//
// TODO(GREEN): RED-stage stub — always errors, never makes an HTTP request.
func runExport(cmd *cobra.Command, deps Deps, conn *connection, blockID, output, out string) error {
	if conn.token() == "" {
		return fmt.Errorf("export: no token configured (--token or %s)", EnvToken)
	}
	return fmt.Errorf("export: not implemented")
}
