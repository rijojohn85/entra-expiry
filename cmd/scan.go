/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/rijojohn85/entra-expiry/internal/graph"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// scanCmd represents the scan command
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logLevel := viper.GetString("loglevel")
		level := slog.LevelInfo
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
		ctx := cmd.Context()
		httpClient := &http.Client{Timeout: 30 * time.Second}
		client, err := graph.NewGraphClient(ctx, "https://graph.microsoft.com/.default", httpClient)
		if err != nil {
			return err
		}
		resp, err := client.DoRequest("https://graph.microsoft.com/v1.0/applications?$select=displayName,appId,passwordCredentials,keyCredentials&$top=1")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("request failed with status code %d", resp.StatusCode)
		}
		slog.Info("Request successful")
		slog.Debug("Response received", "status", resp.Status)
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		slog.Debug("Response Body", "body", string(respBody))
		fmt.Println("scan called")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// scanCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// scanCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	scanCmd.Flags().String("loglevel", "info", "log level (debug, info, warn, error)")
}
