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

// TaskServer implements metarrv1connect.TaskServiceHandler: it turns an
// operator-triggered task into a durable event-bus command. RunDirectoryScan
// resolves the scanner slug, finds the agent that owns the library, and
// publishes an agent.scan command; the scan runs on the agent and its results
// come back on their own stream (internal/server/listeners/agent_scan_result_listener.go).
type TaskServer struct {
	*handlers.Handlers
}

// TaskAuthPolicies is this service's method-name -> policy map. Every task
// method is in the tasks group.
var TaskAuthPolicies = map[string]httpserver.RPCPolicy{
	"RunDirectoryScan": {Group: auth.GroupTasks},
}

func (s *TaskServer) RunDirectoryScan(
	ctx context.Context,
	req *connect.Request[metarrv1.TaskServiceRunDirectoryScanRequest],
) (*connect.Response[metarrv1.TaskServiceRunDirectoryScanResponse], error) {
	correlationID := correlation.FromContext(ctx)
	slug := req.Msg.GetScannerSlug()

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

	if err := s.Bus.Publish(ctx, eventbus.AgentCommandTopic(agent.Slug), eventbus.AgentScanCommandEventName, correlationID, payload); err != nil {
		s.Logger.Error("failed to send scan command to agent", "agent", agent.Slug, "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue task"))
	}

	return connect.NewResponse(&metarrv1.TaskServiceRunDirectoryScanResponse{
		ScanId: correlationID,
	}), nil
}
