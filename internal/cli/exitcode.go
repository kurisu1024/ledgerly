package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// ExitError carries an explicit process exit code alongside the error
// message cobra prints to stderr. Commands that need to distinguish usage
// errors from verification failures (verify: 2 vs 1) return one of these
// instead of a plain error.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// Run executes cmd and returns the process exit code: 0 on success, the
// Code of an *ExitError when a command returns one, or 1 for any other
// error. main.go calls this and passes the result to os.Exit.
func Run(cmd *cobra.Command) int {
	err := cmd.Execute()
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}
