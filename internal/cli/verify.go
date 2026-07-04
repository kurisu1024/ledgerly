package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/kurisu1024/ledgerly/api/events"
	"github.com/kurisu1024/ledgerly/internal/audit"
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
func runVerify(cmd *cobra.Command, deps Deps, path, output string) error {
	if output != "table" && output != "json" {
		return &ExitError{Code: 2, Err: fmt.Errorf("verify: unknown output format %q (want table or json)", output)}
	}

	var input io.Reader = cmd.InOrStdin()
	if path != "" && path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return &ExitError{Code: 2, Err: fmt.Errorf("verify: failed to open input file: %w", err)}
		}
		defer f.Close()
		input = f
	}

	data, err := io.ReadAll(input)
	if err != nil {
		return &ExitError{Code: 2, Err: fmt.Errorf("verify: failed to read input: %w", err)}
	}

	var dtos []events.Event
	if err := json.Unmarshal(data, &dtos); err != nil {
		return &ExitError{Code: 2, Err: fmt.Errorf("verify: failed to parse events JSON: %w", err)}
	}

	cmd.SilenceUsage = true

	results, err := verifyGroupedChains(dtos)
	if err != nil {
		return &ExitError{Code: 2, Err: fmt.Errorf("verify: %w", err)}
	}

	if err := renderVerifyResults(cmd.OutOrStdout(), results, output); err != nil {
		return &ExitError{Code: 2, Err: fmt.Errorf("verify: %w", err)}
	}

	for _, r := range results {
		if r.Status != string(audit.StatusVerified) {
			return &ExitError{Code: 1, Err: fmt.Errorf(
				"verify: chain %s is %s (reason %s, failed-index %d)",
				r.ChainID, r.Status, r.Reason, r.FailedIndex)}
		}
	}
	return nil
}

// verifyGroupedChains groups wire DTOs by chain-id (preserving first-seen
// order), reconstructs each group as an audit.EventChain, and runs
// audit.VerifyChainReport over it. An empty input is reported as a single
// unverifiable result.
func verifyGroupedChains(dtos []events.Event) ([]ChainVerifyResult, error) {
	if len(dtos) == 0 {
		empty := audit.VerifyChainReport(audit.EventChain{})
		return []ChainVerifyResult{{
			Status:      string(empty.Status),
			Reason:      string(empty.Reason),
			FailedIndex: empty.FailedIndex,
			Length:      empty.Length,
		}}, nil
	}

	var order []string
	groups := make(map[string]*audit.EventChain)
	for i, dto := range dtos {
		ae, err := events.ToAuditEvent(dto)
		if err != nil {
			return nil, fmt.Errorf("failed to convert event %d: %w", i, err)
		}
		key := dto.ChainID
		chain, ok := groups[key]
		if !ok {
			order = append(order, key)
			chain = &audit.EventChain{ID: ae.ChainID}
			groups[key] = chain
		}
		chain.Events = append(chain.Events, ae)
	}

	results := make([]ChainVerifyResult, 0, len(order))
	for _, key := range order {
		r := audit.VerifyChainReport(*groups[key])
		results = append(results, ChainVerifyResult{
			ChainID:     key,
			Status:      string(r.Status),
			Reason:      string(r.Reason),
			FailedIndex: r.FailedIndex,
			Length:      r.Length,
		})
	}
	return results, nil
}

// renderVerifyResults writes results as an indented JSON array or a
// tabwriter table.
func renderVerifyResults(w io.Writer, results []ChainVerifyResult, output string) error {
	if output == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "CHAIN-ID\tSTATUS\tREASON\tFAILED-INDEX\tLENGTH")
	for _, r := range results {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\n", r.ChainID, r.Status, r.Reason, r.FailedIndex, r.Length)
	}
	return tw.Flush()
}
