package cmd

import (
	"fmt"

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
running in AKS. Not fully implemented yet.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port := viper.GetInt("port")
		fmt.Printf("starting server on port %d\n", port)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().Int("port", 8080, "Port to run the server on")
}
