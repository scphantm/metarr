// Command metarrctl is a command-line client for the Metarr API.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"Metarr/internal/cliclient"
)

var (
	serverURL  string
	apiKeyFlag string
)

var rootCmd = &cobra.Command{
	Use:   "metarrctl",
	Short: "Command-line client for the Metarr API",
	Long:  "metarrctl drives the Metarr API from the command line: heartbeat, login/logout, application config, background tasks, and the scanned media library.",
}

func main() {
	rootCmd.SilenceUsage = true

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", envOrDefault("METARR_SERVER", "http://localhost:8080"), "Metarr API server URL")
	rootCmd.PersistentFlags().StringVar(&apiKeyFlag, "api-key", os.Getenv("METARR_API_KEY"), "API key (falls back to a saved session from `metarrctl login`)")

	rootCmd.AddCommand(heartbeatCmd, loginCmd, logoutCmd, configCmd, tasksCmd, localDirectoriesCmd, mediaFilesCmd)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// resolvedAPIKey returns the API key to use for an authenticated command:
// the --api-key flag/METARR_API_KEY env var if set, else the credentials
// saved by a prior `metarrctl login` for the current --server.
func resolvedAPIKey() (string, error) {
	if apiKeyFlag != "" {
		return apiKeyFlag, nil
	}
	if key, ok := cliclient.LoadCredentials(serverURL); ok && key != "" {
		return key, nil
	}
	return "", fmt.Errorf("not authenticated: run `metarrctl login`, or pass --api-key / set METARR_API_KEY")
}

// newClient builds a Client for a command that requires authentication.
func newClient() (*cliclient.Client, error) {
	key, err := resolvedAPIKey()
	if err != nil {
		return nil, err
	}
	return cliclient.New(serverURL, key), nil
}

// newPublicClient builds a Client for a command that doesn't require
// authentication (heartbeat, login) — --api-key is still sent if provided.
func newPublicClient() *cliclient.Client {
	return cliclient.New(serverURL, apiKeyFlag)
}

// printJSON pretty-prints v to stdout.
func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
