package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

var (
	logsFollow bool
	logsLines  int
)

var logsCmd = &cobra.Command{
	Use:   "logs [service|agent]",
	Short: "Tail logs from a service or agent",
	Long: `Stream or tail logs from any service or agent.

Examples:
  angee logs web              # tail web service logs
  angee logs admin            # tail admin agent logs
  angee logs web --follow     # follow web logs live
  angee logs --lines 50       # last 50 lines (all services)`,
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", 100, "Number of lines to show")
}

func runLogs(cmd *cobra.Command, args []string) error {
	if !isOperatorRunning() {
		return fmt.Errorf("operator not running — start with 'angee up'")
	}

	service := ""
	if len(args) > 0 {
		service = args[0]
	}

	// Build URL
	params := url.Values{}
	params.Set("lines", fmt.Sprintf("%d", logsLines))
	if logsFollow {
		params.Set("follow", "true")
	}

	path := "/logs"
	if service != "" {
		path = fmt.Sprintf("/logs/%s", service)
	}

	reqURL := fmt.Sprintf("%s%s?%s", resolveOperator(), path, params.Encode())
	resp, err := http.Get(reqURL)
	if err != nil {
		return fmt.Errorf("fetching logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("error fetching logs (status %d)", resp.StatusCode)
	}

	io.Copy(os.Stdout, resp.Body) //nolint:errcheck
	return nil
}
