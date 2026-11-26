package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/rijojohn85/entra-expiry/internal/provider"
	"github.com/rijojohn85/entra-expiry/internal/report"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan the tenant for expired or expiring credentials",
	Long: `Runs a single scan against your Entra ID tenant via the Microsoft Graph
API, listing every app registration secret and certificate that has
expired or expires within the --days window, and prints a report.

Requires the Application.Read.All application permission and an
authenticated default credential (az login locally).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Flags parsed by here — a usage dump is only useful for flag
		// mistakes, not for runtime failures below.
		cmd.SilenceUsage = true
		ctx := cmd.Context()
		httpClient := &http.Client{Timeout: 30 * time.Second}
		p, err := provider.NewGraphProvider("https://graph.microsoft.com/.default", httpClient)
		if err != nil {
			return err
		}

		return scanRun(ctx, p, os.Stdout, viper.GetInt("days"))
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.Flags().Bool("json", false, "output as json")
}

// appProvider is the consumer-side interface (see
// go.dev/wiki/CodeReviewComments#interfaces): scanRun depends only on this,
// tests use a fake, internal/provider stays concrete.
type appProvider interface {
	ListApplications(ctx context.Context) ([]provider.Application, error)
}

// reporter is the consumer-side seam between scanRun and the output
// formats: the table report and the --json payload both Generate() here.
type reporter interface {
	Generate() error
}

var _ appProvider = (*provider.GraphProvider)(nil)

func scanRun(ctx context.Context, p appProvider, w io.Writer, days int) error {
	apps, err := p.ListApplications(ctx)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	slog.Debug("credential scan successful", "applications", len(apps))
	var rep reporter
	if viper.GetBool("json") {
		rep = report.NewJSONReport(apps, days, w)
	} else {
		rep = report.NewReport(apps, days, w)
	}
	if err := rep.Generate(); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	return nil
}
