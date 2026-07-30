package listeners

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"Metarr/internal/appconfig"
	"Metarr/internal/eventbus"
	"Metarr/internal/mediascan"
	"Metarr/internal/mongostore"
)

// RunDirectoryScanListener consumes directory_scan events off the durable event
// stream. Each event names one configured scan directory; the listener walks its
// immediate subdirectories, scans each as a media item, and replaces the stored
// records for that scan root.
func RunDirectoryScanListener(
	ctx context.Context,
	bus *eventbus.StreamBus,
	repo *mongostore.LocalDirectoryRepo,
	logger *slog.Logger,
) error {
	logger.Info("directory_scan listener started", "stream", eventbus.DirectoryScanStream)

	return bus.Consume(ctx, eventbus.DirectoryScanStream, eventbus.DirectoryScanGroup, "worker-1",
		func(ctx context.Context, event eventbus.Event) error {
			logger.Info("event fired", "event", event.Name, "correlation_id", event.CorrelationID)
			runDirectoryScan(ctx, repo, logger, event)
			return nil
		})
}

// runDirectoryScan performs one scan.
//
// It deliberately never returns an error. Redis Streams redelivers anything that
// is not acknowledged, so reporting a failure here — an unreadable directory, a
// scan type that is no longer valid — would put the event into an endless
// retry loop. Failures are logged instead and the event is consumed.
func runDirectoryScan(
	ctx context.Context,
	repo *mongostore.LocalDirectoryRepo,
	logger *slog.Logger,
	event eventbus.Event,
) {
	log := logger.With("correlation_id", event.CorrelationID)

	var scanDirectory appconfig.ScanDirectory
	if err := json.Unmarshal(event.Payload, &scanDirectory); err != nil {
		log.Error("failed to decode directory_scan payload", "error", err)
		return
	}

	directoryType, err := mediascan.ParseDirectoryType(scanDirectory.ScanType)
	if err != nil {
		log.Error("directory_scan has an unusable scan type",
			"scanner_slug", scanDirectory.ScannerSlug, "scan_type", scanDirectory.ScanType, "error", err)
		return
	}

	scanRootPath, err := filepath.Abs(scanDirectory.Directory)
	if err != nil {
		log.Error("failed to resolve scan directory", "directory", scanDirectory.Directory, "error", err)
		return
	}

	itemDirectories, err := readItemDirectories(scanRootPath)
	if err != nil {
		log.Error("failed to read scan directory", "directory", scanRootPath, "error", err)
		return
	}

	// The timestamp is taken before any scanning so that every record written by
	// this run compares as newer than it, and the stale sweep afterwards removes
	// exactly what this run did not touch.
	scanStartedAt := time.Now().UTC()

	log.Info("directory scan starting",
		"scanner_slug", scanDirectory.ScannerSlug,
		"directory", scanRootPath,
		"type", directoryType,
		"item_directories", len(itemDirectories))

	results := scanItemDirectories(itemDirectories, directoryType, parallelCount(), log)

	if err := repo.ReplaceScanResults(ctx, scanRootPath, results, scanStartedAt); err != nil {
		log.Error("failed to store directory scan results", "directory", scanRootPath, "error", err)
		return
	}

	mediaFileCount := 0
	for _, result := range results {
		mediaFileCount += len(result.MediaFiles)
	}
	log.Info("directory scan finished",
		"scanner_slug", scanDirectory.ScannerSlug,
		"directory", scanRootPath,
		"directories_stored", len(results),
		"media_files_stored", mediaFileCount)
}

// readItemDirectories lists the immediate subdirectories of a scan root. Each one
// is a single movie, series, or music video.
func readItemDirectories(scanRootPath string) ([]string, error) {
	entries, err := os.ReadDir(scanRootPath)
	if err != nil {
		return nil, err
	}

	directories := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directories = append(directories, filepath.Join(scanRootPath, entry.Name()))
	}
	return directories, nil
}

// scanItemDirectories scans up to parallelCount item directories at a time.
//
// A semaphore and a WaitGroup are used rather than errgroup because a single
// unreadable item directory must not cancel the rest of the library: the failure
// is logged and the remaining directories still get scanned.
func scanItemDirectories(
	itemDirectories []string,
	directoryType mediascan.DirectoryType,
	parallelCount int,
	log *slog.Logger,
) []*mediascan.ScanResult {
	var (
		waitGroup    sync.WaitGroup
		resultsMutex sync.Mutex
		results      = make([]*mediascan.ScanResult, 0, len(itemDirectories))
		semaphore    = make(chan struct{}, parallelCount)
	)

	for _, itemDirectory := range itemDirectories {
		waitGroup.Add(1)
		go func(directory string) {
			defer waitGroup.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result, err := mediascan.Scan(directory, directoryType)
			if err != nil {
				log.Error("failed to scan item directory", "directory", directory, "error", err)
				return
			}
			for _, warning := range result.Directory.Warnings {
				log.Warn("scan warning", "directory", directory, "warning", warning)
			}

			resultsMutex.Lock()
			results = append(results, result)
			resultsMutex.Unlock()
		}(itemDirectory)
	}

	waitGroup.Wait()
	return results
}

// parallelCount reads the configured concurrency from the in-memory config
// singleton, clamped to at least one so a misconfigured zero can't stall the
// scan.
func parallelCount() int {
	count := appconfig.Get().DirectoryScanner.ParallelCount
	if count < 1 {
		return 1
	}
	return count
}
