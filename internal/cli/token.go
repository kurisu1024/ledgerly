package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// maxTokenTTL mirrors api/http's maxTokenLifetime — the server rejects any
// token whose exp-iat exceeds this, so the CLI refuses to mint one too
// rather than handing the caller a token guaranteed to be rejected.
const maxTokenTTL = 24 * time.Hour

func newTokenCmd(deps Deps) *cobra.Command {
	var tenantID string
	var ttl time.Duration

	cmd := &cobra.Command{
		Use:   "token",
		Short: "Generate a development JWT for authenticating with the Ledgerly API",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runToken(cmd, deps, tenantID, ttl)
		},
	}

	cmd.Flags().StringVar(&tenantID, "tenant-id", "", "Tenant ID to embed in the token's tenant_id claim (defaults to a random UUID)")
	cmd.Flags().DurationVar(&ttl, "ttl", time.Hour, "Token lifetime; exp = iat + ttl (max 24h)")

	return cmd
}

// runToken mints a dev JWT (RS256 header, snake_case tenant_id claim, iat
// from deps.Now, exp = iat+ttl) and writes the bare token plus a trailing
// newline to cmd.OutOrStdout(). It rejects ttl > maxTokenTTL.
//
// TODO(GREEN): RED-stage stub — always errors, never signs or writes a
// token.
func runToken(cmd *cobra.Command, deps Deps, tenantID string, ttl time.Duration) error {
	if ttl > maxTokenTTL {
		return fmt.Errorf("--ttl %s exceeds the maximum token lifetime of %s", ttl, maxTokenTTL)
	}
	return fmt.Errorf("token: not implemented")
}
