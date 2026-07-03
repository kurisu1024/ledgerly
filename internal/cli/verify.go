package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ChainVerifyResult is the JSON shape verify -o json emits per chain,
// kebab-case to stay aligned with the web viewer's client-side VerifyResult.
type ChainVerifyResult struct {
	ChainID     string `json:"chain-id"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
	FailedIndex int    `json:"failed-index"`
	Length      int    `json:"length"`
}

func newVerifyCmd(deps Deps) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "verify [file|-]",
		Short: "Verify hash-chain integrity of an exported events JSON array",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			return runVerify(cmd, deps, path, output)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table|json")

	return cmd
}

// runVerify reads a JSON array of api/events.Event (from path, or stdin when
// path is "-" or empty), groups events by chain-id preserving input order,
// reconstructs each group as an audit.EventChain, and runs
// audit.VerifyChainReport over it. Exit 0 when every chain verifies, exit 1
// when any chain is tampered or unverifiable (naming the chain-id,
// failed-index, and reason), exit 2 on malformed JSON or other usage
// errors. An empty input array is itself reported as a single unverifiable
// result.
//
// TODO(GREEN): RED-stage stub — always returns a usage-level ExitError,
// never reads input or calls audit.VerifyChainReport.
func runVerify(cmd *cobra.Command, deps Deps, path, output string) error {
	return &ExitError{Code: 2, Err: fmt.Errorf("verify: not implemented")}
}
