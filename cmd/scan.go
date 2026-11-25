package cmd

import (
	"context"
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
}

type appProvider interface {
	ListApplications(ctx context.Context) ([]provider.Application, error)
}
type Output interface {
	Generate()
}

func scanRun(ctx context.Context, p appProvider, w io.Writer, days int) error {
	apps, err := p.ListApplications(ctx)
	if err != nil {
		return err
	}
	slog.Debug("Credential Scan successful", "applications", len(apps))
	reportOutput := report.NewReport(apps, days, w)
	outputTable(reportOutput)
	return nil
}

func outputTable(report Output) {
	report.Generate()
}
