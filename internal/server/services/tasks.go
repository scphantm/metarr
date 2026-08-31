package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/scanmodel"
)

// TaskServer implements metarrv1connect.TaskServiceHandler, ported directly
// from internal/server/handlers/tasks.go — same event-bus publish calls, only
// the transport changed. The async listeners on the other end are
// unaffected by this migration.
type TaskServer struct {
	*handlers.Handlers
}

// TaskAuthPolicies is this service's method-name -> policy map. Mirrors
// every task route in router.go being GroupTasks.
var TaskAuthPolicies = map[string]httpserver.RPCPolicy{
	"RunDirectoryScan": {Group: auth.GroupTasks},
}

func (s *TaskServer) RunDirectoryScan(
	ctx context.Context,
	req *connect.Request[metarrv1.TaskServiceRunDirectoryScanRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)
	slug := req.Msg.GetScannerSlug()

	if req.Msg.GetCommand() != "run" {
		return nil, connectError(http.StatusBadRequest, errors.New(`unsupported command, expected "run"`))
	}

	appConfig := appconfig.Get()

	index := appconfig.FindScanDirectoryIndex(appConfig.DirectoryScanner, slug)
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no scan directory with that slug"))
	}
	scanDirectory := appConfig.DirectoryScanner.ScanDirectories[index]

	if _, err := scanmodel.ParseDirectoryType(scanDirectory.ScanType); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	// Scanning happens on the agent that can actually see the library, so the
	// request has to resolve to one before anything is queued. Firing at a
	// stream nobody reads would return accepted and then silently do nothing.
	agent, mapped := appconfig.AgentForScanner(appConfig, slug)
	if !mapped {
		return nil, connectError(http.StatusUnprocessableEntity,
			fmt.Errorf("no agent is mapped to scan directory %q; map one under System > Agents", slug))
	}

	// No point-in-time online check here. The command stream is durable, so a
	// command dispatched to an agent that is briefly absent waits on the
	// stream and runs when the agent returns; one that never returns is
	// failed by the presence watcher's "agent offline" signal. Checking
	// presence here and firing separately only reopened a time-of-check-to-
	// time-of-use gap between the two (docs/adr/0006).

	payload, err := json.Marshal(agentproto.ScanCommand{
		ScanID:      correlationID,
		ScannerSlug: slug,
	})
	if err != nil {
		s.Logger.Error("failed to encode scan command", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue task"))
	}

	event := eventbus.NewEvent(eventbus.SourceServer, eventbus.AgentScanCommandEventName, correlationID, payload)

	if err := s.Streams.Publish(ctx, eventbus.AgentCommandTopic(agent.Slug), event); err != nil {
		s.Logger.Error("failed to send scan command to agent", "agent", agent.Slug, "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue task"))
	}

	return connect.NewResponse(&metarrv1.AcceptedResponse{
		Status:        "accepted",
		Event:         eventbus.AgentScanCommandEventName,
		CorrelationId: correlationID,
	}), nil
}
