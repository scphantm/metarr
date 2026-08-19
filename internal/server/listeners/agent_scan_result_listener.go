package listeners

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"Metarr/internal/server/agentregistry"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/eventbus"
)

// RunAgentScanResultListener consumes what the agents report from their scans
// and persists it.
//
// This is the half of the old in-process directory scan that stayed on the
// server. The agent walks the filesystem and sends what it found; nothing here
// touches a disk. Results arrive per item directory as the agent finds them, so
// the library fills in progressively, and the sweep that removes what the scan
// no longer found runs only when the agent reports it finished.
func RunAgentScanResultListener(
	ctx context.Context,
	bus *eventbus.StreamBus,
	repo *mongostore.LocalDirectoryRepo,
	logger *slog.Logger,
) error {
	logger.Info("agent scan result listener started", "stream", agentproto.ScanResultStream)

	return bus.Consume(
		ctx,
		agentproto.ScanResultStream,
		agentproto.ScanResultGroup,
		"worker-1",
		func(ctx context.Context, event eventbus.Event) error {
			return handleAgentScanEvent(ctx, repo, logger, event)
		},
	)
}

func handleAgentScanEvent(
	ctx context.Context,
	repo *mongostore.LocalDirectoryRepo,
	logger *slog.Logger,
	event eventbus.Event,
) error {
	switch event.Name {
	case agentproto.ScanResultEventName:
		return storeScanResult(ctx, repo, logger, event)
	case agentproto.ScanCompleteEventName:
		return completeScan(ctx, repo, logger, event)
	case agentproto.ScanFailedEventName:
		reportScanFailure(logger, event)
		return nil
	default:
		logger.Warn("ignoring unknown agent scan event", "event", event.Name)
		return nil
	}
}

func storeScanResult(
	ctx context.Context,
	repo *mongostore.LocalDirectoryRepo,
	logger *slog.Logger,
	event eventbus.Event,
) error {
	var message agentproto.ScanResultMessage
	if err := json.Unmarshal(event.Payload, &message); err != nil {
		// A payload that will never decode must not be retried forever, so it
		// is logged and acknowledged rather than returned.
		logger.Error("could not decode agent scan result", "error", err)
		return nil
	}

	log := logger.With(
		"agent", message.AgentSlug,
		"scanner_slug", message.ScannerSlug,
		"scan_id", message.ScanID,
	)

	translator, err := translatorFor(message.AgentSlug, message.ScannerSlug)
	if err != nil {
		log.Error("cannot store scan result", "error", err)
		return nil
	}

	// Everything the agent sent is in its own filesystem's terms. It has to be
	// rewritten before it is stored, or the library ends up holding paths that
	// only mean something on another machine.
	if err := translator.Result(message.Result); err != nil {
		log.Error("rejecting scan result with untranslatable paths", "error", err)
		return nil
	}

	serverRoot, err := translator.Path(message.ScanRootPath)
	if err != nil {
		log.Error("rejecting scan result with an unexpected scan root",
			"scan_root", message.ScanRootPath, "error", err)
		return nil
	}

	// A storage failure is returned, so the message is redelivered: unlike a
	// malformed payload, this is the kind of error that succeeds on a retry.
	if err := repo.UpsertScanResult(ctx, serverRoot, message.Result); err != nil {
		log.Error("failed to store scan result", "error", err)
		return err
	}

	return nil
}

func completeScan(
	ctx context.Context,
	repo *mongostore.LocalDirectoryRepo,
	logger *slog.Logger,
	event eventbus.Event,
) error {
	var message agentproto.ScanCompleteMessage
	if err := json.Unmarshal(event.Payload, &message); err != nil {
		logger.Error("could not decode agent scan completion", "error", err)
		return nil
	}

	log := logger.With(
		"agent", message.AgentSlug,
		"scanner_slug", message.ScannerSlug,
		"scan_id", message.ScanID,
	)

	translator, err := translatorFor(message.AgentSlug, message.ScannerSlug)
	if err != nil {
		log.Error("cannot complete scan", "error", err)
		return nil
	}

	serverRoot, err := translator.Path(message.ScanRootPath)
	if err != nil {
		log.Error("cannot complete scan", "scan_root", message.ScanRootPath, "error", err)
		return nil
	}

	// Only now, and only for a scan that ran to the end. Sweeping on a scan
	// that failed part way would delete the half the agent never reached.
	if err := repo.DeleteStaleRecords(ctx, serverRoot, message.StartedAt); err != nil {
		log.Error("failed to remove records the scan no longer found", "error", err)
		return err
	}

	log.Info("scan complete",
		"directory", serverRoot,
		"items", message.ItemCount,
		"duration", message.FinishedAt.Sub(message.StartedAt).String(),
	)
	return nil
}

func reportScanFailure(logger *slog.Logger, event eventbus.Event) {
	var message agentproto.ScanFailedMessage
	if err := json.Unmarshal(event.Payload, &message); err != nil {
		logger.Error("could not decode agent scan failure", "error", err)
		return
	}

	// Deliberately not swept: a failed scan leaves the previous records in
	// place rather than deleting a library because an agent could not read it.
	logger.Error("agent reported a failed scan",
		"agent", message.AgentSlug,
		"scanner_slug", message.ScannerSlug,
		"scan_id", message.ScanID,
		"error", message.Error,
	)
}

// translatorFor builds the path translator for one agent's view of one library,
// reading the current mapping from the in-memory config.
func translatorFor(agentSlug, scannerSlug string) (agentregistry.PathTranslator, error) {
	config := appconfig.Get()

	index := config.FindAgentIndex(agentSlug)
	if index < 0 {
		return agentregistry.PathTranslator{}, errUnknownAgent(agentSlug)
	}

	mapping, ok := config.Agents[index].FindMapping(scannerSlug)
	if !ok {
		return agentregistry.PathTranslator{}, errUnmappedScanner(agentSlug, scannerSlug)
	}

	scannerIndex := config.DirectoryScanner.FindScanDirectoryIndex(scannerSlug)
	if scannerIndex < 0 {
		return agentregistry.PathTranslator{}, errUnknownScanner(scannerSlug)
	}

	return agentregistry.NewPathTranslator(
		mapping.AgentPath,
		config.DirectoryScanner.ScanDirectories[scannerIndex].Directory,
	), nil
}

// These describe a result that arrived after the configuration behind it
// changed — an agent deleted mid-scan, a mapping removed. The scan is
// abandoned rather than guessed at, since there is no longer a defensible
// answer to where its records belong.
func errUnknownAgent(slug string) error {
	return fmt.Errorf("agent %q is no longer configured", slug)
}

func errUnmappedScanner(agentSlug, scannerSlug string) error {
	return fmt.Errorf("%q is no longer mapped to agent %q", scannerSlug, agentSlug)
}

func errUnknownScanner(slug string) error {
	return fmt.Errorf("scan directory %q no longer exists", slug)
}
