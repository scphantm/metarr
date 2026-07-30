package main

import "github.com/spf13/cobra"

var tasksCmd = &cobra.Command{
	Use:   "tasks",
	Short: "Trigger background jobs",
}

var sonarrCacheDataCmd = &cobra.Command{
	Use:   "sonarr-cache-data",
	Short: "sonarr_cache_data background job",
}

var sonarrCacheDataRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Trigger the sonarr_cache_data background job",
	Long:  "Fires the sonarr_cache_data event onto the durable event stream in a non-blocking way and returns as soon as the event has been queued.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.TriggerSonarrCacheData(cmd.Context())
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var directoryScanCmd = &cobra.Command{
	Use:   "directory-scan",
	Short: "directory_scan background job",
}

var directoryScanRunCmd = &cobra.Command{
	Use:   "run <slug>",
	Short: "Scan a configured directory",
	Long: "Scans the configured scan directory with the given scanner_slug, storing one record per media item directory " +
		"plus one per media file. Fires the directory_scan event onto the durable event stream and returns as soon as it has been queued.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.TriggerDirectoryScan(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

func init() {
	tasksCmd.AddCommand(sonarrCacheDataCmd, directoryScanCmd)
	sonarrCacheDataCmd.AddCommand(sonarrCacheDataRunCmd)
	directoryScanCmd.AddCommand(directoryScanRunCmd)
}
