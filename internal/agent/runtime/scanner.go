package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"Metarr/internal/agent/mediascan"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/scanmodel"
)

// maxResultBytes caps one encoded result message.
//
// A single item directory is usually small, but a long-running series with
// trickplay tiles produces hundreds of sidecar records, and a stream entry
// holding all of it is both slow to move and awkward to recover from. Oversized
// items are split across numbered parts instead.
const maxResultBytes = 256 * 1024

// Scanner walks the libraries this agent has been mapped to and streams the
// results back to the server.
type Scanner struct {
	bus    *eventbus.Bus
	config *ConfigStore
	logger *slog.Logger
	slug   string
}

// NewScanner returns a Scanner for one agent.
func NewScanner(bus *eventbus.Bus, config *ConfigStore, logger *slog.Logger, slug string) *Scanner {
	return &Scanner{bus: bus, config: config, logger: logger, slug: slug}
}

// Register wires the scan-command consumer onto the agent's Bus. The command
// topic is per-slug; per-(topic, name) dispatch and the unknown-name default
// live in the bus, so this is one handler map with one entry.
func (s *Scanner) Register(bus *eventbus.Bus) error {
	return bus.HandleStream(eventbus.AgentCommandTopic(s.slug), map[string]eventbus.StreamHandler{
		eventbus.AgentScanCommandEventName: s.handle,
	})
}

func (s *Scanner) handle(ctx context.Context, event *eventbus.Event) error {
	var command agentproto.ScanCommand
	if err := json.Unmarshal(event.Payload, &command); err != nil {
		// An undecodable command cannot be processed at all: let the Router
		// retry and then park it, rather than silently swallowing it.
		return fmt.Errorf("decode scan command: %w", err)
	}

	log := s.logger.With(
		"scan_id", command.ScanID,
		"scanner_slug", command.ScannerSlug,
		"correlation_id", event.CorrelationId,
	)

	if err := s.scan(ctx, command, log); err != nil {
		// The scan ran and failed for a reason a retry will not change — an
		// unmapped slug, an unreadable root. It is reported to the server as a
		// failure result event and the handler returns nil, so a business
		// failure never enters the retry path.
		log.Error("scan failed", "error", err)
		if reportErr := s.report(ctx, event.CorrelationId, eventbus.AgentScanFailedEventName, agentproto.ScanFailedMessage{
			ScanID:      command.ScanID,
			AgentSlug:   s.slug,
			ScannerSlug: command.ScannerSlug,
			Error:       err.Error(),
		}); reportErr != nil {
			log.Error("failed to report scan failure", "error", reportErr)
		}
	}
	return nil
}

func (s *Scanner) scan(ctx context.Context, command agentproto.ScanCommand, log *slog.Logger) error {
	projection := s.config.Current()
	if projection == nil {
		return fmt.Errorf("this agent has no configuration yet")
	}

	mapped, ok := agentproto.FindDirectory(projection, command.ScannerSlug)
	if !ok {
		return fmt.Errorf("%q is not mapped to this agent", command.ScannerSlug)
	}

	directoryType, err := scanmodel.ParseDirectoryType(mapped.ScanType)
	if err != nil {
		return err
	}

	scanRootPath, err := filepath.Abs(mapped.AgentPath)
	if err != nil {
		return err
	}

	itemDirectories, err := readItemDirectories(scanRootPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", scanRootPath, err)
	}

	// Taken before the walk, so anything the server holds under this root that
	// is older than this timestamp is something the scan did not find and can
	// be swept. Taking it afterwards would delete files created during a long
	// scan.
	startedAt := time.Now().UTC()

	log.Info("scan started", "directory", scanRootPath, "items", len(itemDirectories))

	sent := s.scanAndStream(ctx, scanStreamRequest{
		command:       command,
		scanRootPath:  scanRootPath,
		directoryType: directoryType,
		itemPaths:     itemDirectories,
		parallelCount: parallelCountFrom(projection),
		startedAt:     startedAt,
		log:           log,
	})

	log.Info("scan finished", "items_sent", sent)

	return s.report(ctx, command.ScanID, eventbus.AgentScanCompleteEventName, agentproto.ScanCompleteMessage{
		ScanID:       command.ScanID,
		AgentSlug:    s.slug,
		ScannerSlug:  command.ScannerSlug,
		ScanRootPath: scanRootPath,
		ItemCount:    sent,
		StartedAt:    startedAt,
		FinishedAt:   time.Now().UTC(),
	})
}

type scanStreamRequest struct {
	command       agentproto.ScanCommand
	scanRootPath  string
	directoryType scanmodel.DirectoryType
	itemPaths     []string
	parallelCount int
	startedAt     time.Time
	log           *slog.Logger
}

// scanAndStream scans up to parallelCount item directories at a time, emitting
// each result as it completes.
//
// A semaphore and a WaitGroup rather than errgroup, because one unreadable item
// directory must not cancel the rest of the library: it is logged and the
// remaining directories still get scanned. Results are emitted as they finish
// rather than collected, so the server's copy of the library fills in
// progressively and a very large scan never has to sit in memory whole.
func (s *Scanner) scanAndStream(ctx context.Context, request scanStreamRequest) int {
	var (
		waitGroup sync.WaitGroup
		sentMutex sync.Mutex
		sent      int
		semaphore = make(chan struct{}, request.parallelCount)
	)

	for _, itemPath := range request.itemPaths {
		if ctx.Err() != nil {
			break
		}

		waitGroup.Add(1)
		go func(directory string) {
			defer waitGroup.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result, err := mediascan.Scan(directory, request.directoryType)
			if err != nil {
				request.log.Error("failed to scan item directory", "directory", directory, "error", err)
				return
			}
			for _, warning := range result.Directory.Warnings {
				request.log.Warn("scan warning", "directory", directory, "warning", warning)
			}

			if err := s.sendResult(ctx, request, result); err != nil {
				request.log.Error("failed to send scan result", "directory", directory, "error", err)
				return
			}

			sentMutex.Lock()
			sent++
			sentMutex.Unlock()
		}(itemPath)
	}

	waitGroup.Wait()
	return sent
}

// sendResult emits one item, splitting it across parts when it is too large to
// travel in a single stream entry.
func (s *Scanner) sendResult(ctx context.Context, request scanStreamRequest, result *scanmodel.ScanResult) error {
	base := agentproto.ScanResultMessage{
		ScanID:       request.command.ScanID,
		AgentSlug:    s.slug,
		ScannerSlug:  request.command.ScannerSlug,
		ScanRootPath: request.scanRootPath,
		Result:       result,
		PartIndex:    0,
		PartCount:    1,
		ScannedAt:    request.startedAt,
	}

	encoded, err := json.Marshal(base)
	if err != nil {
		return err
	}
	if len(encoded) <= maxResultBytes {
		return s.publish(ctx, request.command.ScanID, eventbus.AgentScanResultEventName, encoded)
	}

	return s.sendResultInParts(ctx, request, result, base, len(encoded))
}

// sendResultInParts splits an oversized item across several messages. The
// directory record travels with part 0; later parts carry only additional media
// files, which the server appends.
func (s *Scanner) sendResultInParts(
	ctx context.Context,
	request scanStreamRequest,
	result *scanmodel.ScanResult,
	base agentproto.ScanResultMessage,
	encodedSize int,
) error {
	files := result.MediaFiles
	if len(files) <= 1 {
		// Nothing left to split on: one enormous record has to go as it is.
		encoded, err := json.Marshal(base)
		if err != nil {
			return err
		}
		return s.publish(ctx, request.command.ScanID, eventbus.AgentScanResultEventName, encoded)
	}

	// Size the chunks from what the whole item actually encoded to, rather than
	// guessing a file count that would be wrong for both a series of shorts and
	// a series of feature-length episodes.
	perFile := encodedSize / len(files)
	chunkSize := max(1, maxResultBytes/max(perFile, 1))
	partCount := (len(files) + chunkSize - 1) / chunkSize

	request.log.Info("splitting oversized scan result",
		"directory", result.Directory.Path,
		"media_files", len(files),
		"parts", partCount,
	)

	for part := range partCount {
		start := part * chunkSize
		end := min(start+chunkSize, len(files))

		message := base
		message.PartIndex = part
		message.PartCount = partCount

		if part == 0 {
			trimmed := *result
			trimmed.MediaFiles = files[start:end]
			message.Result = &trimmed
		} else {
			// Later parts carry no directory record — resending it would mean
			// the server upserting the same document once per part.
			message.Result = &scanmodel.ScanResult{MediaFiles: files[start:end]}
		}

		encoded, err := json.Marshal(message)
		if err != nil {
			return err
		}
		if err := s.publish(ctx, request.command.ScanID, eventbus.AgentScanResultEventName, encoded); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scanner) publish(ctx context.Context, correlationID, name string, payload []byte) error {
	return s.bus.Publish(ctx, eventbus.AgentScanResultTopic(), name, correlationID, payload)
}

func (s *Scanner) report(ctx context.Context, correlationID, name string, message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return s.publish(ctx, correlationID, name, payload)
}

// readItemDirectories lists the immediate subdirectories of a scan root. Each
// one is a single movie, series, or music video.
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

// parallelCountFrom clamps the configured concurrency to at least one, so a
// misconfigured zero cannot stall the scan.
func parallelCountFrom(projection *agentproto.AgentConfigProjection) int {
	if projection.ParallelCount < 1 {
		return 1
	}
	return int(projection.ParallelCount)
}
