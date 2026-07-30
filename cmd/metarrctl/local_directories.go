package main

import (
	"github.com/spf13/cobra"

	"Metarr/internal/cliclient"
	"Metarr/internal/mediascan"
)

var localDirectoriesCmd = &cobra.Command{
	Use:   "local-directories",
	Short: "Inspect scanned media directories",
	Long:  "Reads the directory and media file records produced by directory scans from the local_directory collection.",
}

var (
	localDirectoriesType     string
	localDirectoriesScanRoot string
	localDirectoriesLimit    int
	localDirectoriesSkip     int
)

var localDirectoriesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List scanned directories",
	Long:  "Returns the scanned directory records, optionally filtered by media type (" + mediascan.ValidDirectoryTypesText() + ") and by the scan root they were found under.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.ListLocalDirectories(cmd.Context(), cliclient.ListLocalDirectoriesRequest{
			Type:     localDirectoriesType,
			ScanRoot: localDirectoriesScanRoot,
			Limit:    localDirectoriesLimit,
			Skip:     localDirectoriesSkip,
		})
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var localDirectoriesGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Fetch a single scanned directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.GetLocalDirectory(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var localDirectoriesMediaFilesCmd = &cobra.Command{
	Use:   "media-files <id>",
	Short: "List a directory's media files",
	Long:  "Returns the media file records — the movie, episode or music video files themselves — belonging to one scanned directory.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.ListDirectoryMediaFiles(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var localDirectoriesNFOPath string

var localDirectoriesNFOCmd = &cobra.Command{
	Use:   "nfo <id>",
	Short: "Read an NFO file from disk",
	Long:  "Reads and parses one .nfo file inside a scanned directory, live from disk rather than from the scan snapshot. --path is relative to the directory and may not escape it.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.GetLocalDirectoryNFO(cmd.Context(), args[0], localDirectoriesNFOPath)
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

var mediaFilesCmd = &cobra.Command{
	Use:   "media-files",
	Short: "Inspect scanned media files",
}

var mediaFilesGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Fetch a single media file record",
	Long:  "Returns one media file record, including its own NFO metadata, subtitles, artwork and episode ids.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		resp, err := client.GetMediaFile(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return printJSON(resp)
	},
}

func init() {
	localDirectoriesCmd.AddCommand(
		localDirectoriesListCmd,
		localDirectoriesGetCmd,
		localDirectoriesMediaFilesCmd,
		localDirectoriesNFOCmd,
	)

	localDirectoriesListCmd.Flags().StringVar(&localDirectoriesType, "type", "", "Filter by media type ("+mediascan.ValidDirectoryTypesText()+")")
	localDirectoriesListCmd.Flags().StringVar(&localDirectoriesScanRoot, "scan-root", "", "Filter by the scan directory these were found under")
	localDirectoriesListCmd.Flags().IntVar(&localDirectoriesLimit, "limit", 0, "Maximum records to return (server default 100, max 500)")
	localDirectoriesListCmd.Flags().IntVar(&localDirectoriesSkip, "skip", 0, "Records to skip")

	localDirectoriesNFOCmd.Flags().StringVar(&localDirectoriesNFOPath, "path", "", "Path to the .nfo file, relative to the directory")
	_ = localDirectoriesNFOCmd.MarkFlagRequired("path")

	mediaFilesCmd.AddCommand(mediaFilesGetCmd)
}
