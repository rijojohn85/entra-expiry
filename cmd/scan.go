package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rijojohn85/entra-expiry/internal/provider"
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

func scanRun(ctx context.Context, p appProvider, w io.Writer, days int) error {
	apps, err := p.ListApplications(ctx)
	if err != nil {
		return err
	}
	slog.Debug("Credential Scan successful", "applications", len(apps))
	outputTable(apps, w, days)
	return nil
}

func outputTable(apps []provider.Application, w io.Writer, days int) {
	t := table.NewWriter()

	t.AppendHeader(table.Row{"AppName", "Cred Kind", "Name", "Expires", "IN(days)", "STATUS"})
	now := time.Now()
	for _, app := range apps {
		for _, cred := range app.Credentials {
			var status string
			switch {
			case cred.Expires.Before(now):
				status = "Expired"
			case cred.Expires.Before(now.Add(time.Duration(days) * 24 * time.Hour)):
				status = "Expiring Soon"
			default:
				status = "Valid"
			}
			in := int(cred.Expires.Sub(now).Hours() / 24)
			kind := cred.Type.String()
			t.AppendRow(table.Row{app.DisplayName, kind, cred.Name, cred.Expires, in, status})
			t.AppendSeparator()
		}
	}
	fmt.Fprintln(w, t.Render())
}
