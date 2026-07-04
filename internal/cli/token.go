package cli

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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
// The token is deliberately unsigned — a dummy signature stands in for the
// third segment, matching the dev token `make load-events` crafts. It only
// works against servers that skip signature verification.
func runToken(cmd *cobra.Command, deps Deps, tenantID string, ttl time.Duration) error {
	if ttl > maxTokenTTL {
		return fmt.Errorf("--ttl %s exceeds the maximum token lifetime of %s", ttl, maxTokenTTL)
	}
	if tenantID == "" {
		tenantID = uuid.NewString()
	}

	now := deps.Now()
	claims := jwt.MapClaims{
		"tenant_id": tenantID,
		"iat":       now.Unix(),
		"exp":       now.Add(ttl).Unix(),
	}

	signingString, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SigningString()
	if err != nil {
		return fmt.Errorf("failed to encode token: %w", err)
	}
	signature := base64.RawURLEncoding.EncodeToString([]byte("dummy-signature"))

	fmt.Fprintln(cmd.OutOrStdout(), signingString+"."+signature)
	return nil
}
