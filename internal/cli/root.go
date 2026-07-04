// Package cli implements the ledgerly-cli commands: token, events post,
// export, and verify. Every command takes an injectable Deps so the real
// binary and tests share the exact same command wiring — tests supply a fake
// HTTPClient/Now and use cobra's SetIn/SetOut/SetErr instead of the process's
// real stdio.
package cli

import (
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// Deps are the injectable dependencies every ledgerly-cli command uses so
// command logic never reaches for globals (http.DefaultClient, time.Now)
// directly.
type Deps struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

// DefaultDeps returns the Deps used by the real binary.
func DefaultDeps() Deps {
	return Deps{
		HTTPClient: http.DefaultClient,
		Now:        time.Now,
	}
}

// Environment variable names backing --server-url and --token when the flag
// is not set.
const (
	EnvServerURL = "LEDGERLY_SERVER_URL"
	EnvToken     = "LEDGERLY_TOKEN"
)

// connection holds the resolved --server-url/--token flag values shared by
// subcommands that talk to the API (events, export). token generates
// credentials and doesn't need them.
type connection struct {
	serverURLFlag string
	tokenFlag     string
}

// serverURL resolves --server-url, falling back to LEDGERLY_SERVER_URL.
// Flag wins over environment.
func (c *connection) serverURL() string {
	if c.serverURLFlag != "" {
		return c.serverURLFlag
	}
	return os.Getenv(EnvServerURL)
}

// token resolves --token, falling back to LEDGERLY_TOKEN. Flag wins over
// environment.
func (c *connection) token() string {
	if c.tokenFlag != "" {
		return c.tokenFlag
	}
	return os.Getenv(EnvToken)
}

// NewRootCmd builds the ledgerly-cli root command with all subcommands
// wired to deps.
func NewRootCmd(deps Deps) *cobra.Command {
	conn := &connection{}

	root := &cobra.Command{
		Use:   "ledgerly-cli",
		Short: "Command-line client for the Ledgerly audit-log API",
	}

	root.PersistentFlags().StringVar(&conn.serverURLFlag, "server-url", "",
		"Ledgerly server base URL (env "+EnvServerURL+")")
	root.PersistentFlags().StringVar(&conn.tokenFlag, "token", "",
		"Bearer token for authentication (prefer env "+EnvToken+" on shared hosts; flag values leak into shell history and process lists)")

	root.AddCommand(newTokenCmd(deps))
	root.AddCommand(newEventsCmd(deps, conn))
	root.AddCommand(newExportCmd(deps, conn))
	root.AddCommand(newVerifyCmd(deps))

	return root
}
