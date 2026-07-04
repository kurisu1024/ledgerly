// Command ledgerly-cli is the command-line client for the Ledgerly
// audit-log API. All command logic lives in internal/cli; this file only
// wires real dependencies and maps the result to a process exit code.
package main

import (
	"net/http"
	"os"
	"time"

	"github.com/kurisu1024/ledgerly/internal/cli"
)

func main() {
	deps := cli.Deps{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
	os.Exit(cli.Run(cli.NewRootCmd(deps)))
}
