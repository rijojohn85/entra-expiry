package cmd

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rijojohn85/entra-expiry/internal/provider"
	"github.com/rijojohn85/entra-expiry/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run as a monitored service, rescanning on an interval",
	Long: `Runs entra-expiry as a long-lived service designed for Kubernetes.
It rescans the tenant on an interval, exposes /healthz and /metrics
endpoints, and shuts down gracefully on SIGTERM.

Authentication uses workload identity (federated credential) when
running in AKS, and az login credentials locally.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Flags parsed by here — a usage dump is only useful for flag
		// mistakes, not for runtime failures below.
		cmd.SilenceUsage = true
		port := viper.GetInt("port")
		p, err := provider.NewGraphProvider("https://graph.microsoft.com/.default", &http.Client{Timeout: 30 * time.Second})
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		svc := server.NewServer(
			port,
			viper.GetInt("days"),
			time.Duration(viper.GetInt("interval"))*time.Second,
			p,
		)
		return svc.Run(ctx)
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	// days and interval come from root's persistent flags; port is
	// serve-only. All three are also settable via config.yaml.
	serveCmd.Flags().Int("port", 8080, "port to run the server on")
}
