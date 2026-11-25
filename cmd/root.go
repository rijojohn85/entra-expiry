package cmd

import (
	"errors"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "entra-expiry",
	Short: "Find Entra ID app registration secrets and certificates that are expired or expiring",
	Long: `entra-expiry scans the app registrations in your own Entra ID tenant
and reports every client secret and certificate credential that has
already expired or expires within a configurable window.

Credentials are read as expiry metadata only through the Microsoft Graph
API — secret values are never fetched or stored.

Commands:
  scan    run one scan and print a report
  serve   run as a monitored service, rescanning on an interval

Authentication uses the Azure default credential chain: az login
credentials locally, workload identity when running in AKS.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initializeConfig(cmd); err != nil {
			return err
		}
		setupLogger()
		return nil
	},
}

var cfgFile string

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
func setupLogger() {
	level := slog.LevelInfo
	logLevel := viper.GetString("loglevel")

	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.entra-expiry.yaml)")
	rootCmd.PersistentFlags().String("loglevel", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().Int("days", 30, "number of days to check for expiring credentials")
}

func initializeConfig(cmd *cobra.Command) error {
	viper.SetEnvPrefix("ENTRA_EXPIRY")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)
		viper.AddConfigPath(".")
		viper.AddConfigPath(home + "/.entra-expiry")
		viper.AddConfigPath(home + "/.config/.entra-expiry")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}
	if err := viper.ReadInConfig(); err != nil {
		var nf viper.ConfigFileNotFoundError
		if !errors.As(err, &nf) {
			return err // real problem (bad YAML), propagate
		}
		// no config file: fine, flags/env are enough
	}
	err := viper.BindPFlags(cmd.Flags())
	return err
}
