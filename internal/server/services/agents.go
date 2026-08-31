package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// agentPresenceStreamInterval matches the interval wsbus.Hub.Register used
// for the agents.presence topic in cmd/metarr-server/main.go.
const agentPresenceStreamInterval = 2 * time.Second

// AgentServer implements metarrv1connect.AgentServiceHandler. Every write
// goes through AppConfigStore.Mutate — see internal/server/appconfigstore.
// StreamPresence replaces the agents.presence wsbus topic, reusing the
// exact same Registry.List call List already makes.
type AgentServer struct {
	*handlers.Handlers
}

// AgentAuthPolicies is this service's method-name -> policy map. Mirrors
// every agent route in router.go being GroupConfig.
var AgentAuthPolicies = map[string]httpserver.RPCPolicy{
	"List":           {Group: auth.GroupConfig, ReadOnly: true},
	"StreamPresence": {Group: auth.GroupConfig, ReadOnly: true},
	"Upsert":         {Group: auth.GroupConfig},
	"Delete":         {Group: auth.GroupConfig},
	"SetLogLevel":    {Group: auth.GroupConfig},
}

func (s *AgentServer) List(
	ctx context.Context,
	req *connect.Request[metarrv1.AgentServiceListRequest],
) (*connect.Response[metarrv1.AgentServiceListResponse], error) {
	appConfig := appconfig.Get()

	views, err := s.Agents.List(ctx, appConfig)
	if err != nil {
		s.Logger.Error("failed to list agents", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to list agents"))
	}

	return connect.NewResponse(&metarrv1.AgentServiceListResponse{Agents: views}), nil
}

func (s *AgentServer) StreamPresence(
	ctx context.Context,
	req *connect.Request[metarrv1.AgentServiceStreamPresenceRequest],
	stream *connect.ServerStream[metarrv1.AgentServiceStreamPresenceResponse],
) error {
	ticker := time.NewTicker(agentPresenceStreamInterval)
	defer ticker.Stop()

	for {
		appConfig := appconfig.Get()

		views, err := s.Agents.List(ctx, appConfig)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.Logger.Error("failed to list agents", "error", err)
			return connectError(http.StatusInternalServerError, errors.New("failed to list agents"))
		}

		if err := stream.Send(&metarrv1.AgentServiceStreamPresenceResponse{Agents: views}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *AgentServer) Upsert(
	ctx context.Context,
	req *connect.Request[metarrv1.AgentServiceUpsertRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	entry := req.Msg.GetAgent()
	if entry == nil {
		entry = &appconfig.AgentConfig{}
	} else {
		entry = cloneMsg(entry)
	}

	if err := agentproto.ValidateSlug(entry.Slug); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		if status, err := validateMappings(cfg, entry); err != nil {
			return connectError(status, err)
		}

		if index := appconfig.FindAgentIndex(cfg, entry.Slug); index == -1 {
			cfg.Agents = append(cfg.Agents, entry)
		} else {
			cfg.Agents[index] = entry
		}
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *AgentServer) Delete(
	ctx context.Context,
	req *connect.Request[metarrv1.AgentServiceDeleteRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)
	slug := req.Msg.GetSlug()

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindAgentIndex(cfg, slug)
		if index == -1 {
			return connectError(http.StatusNotFound, errors.New("no agent with that slug"))
		}
		cfg.Agents = append(cfg.Agents[:index], cfg.Agents[index+1:]...)
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	// Drop the published projection immediately rather than waiting for the
	// config listener: a deleted agent should stop being able to read its
	// mapping now, not once the event has been processed.
	if err := s.Agents.Forget(ctx, slug); err != nil {
		s.Logger.Warn("could not remove the agent's published configuration", "agent", slug, "error", err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *AgentServer) SetLogLevel(
	ctx context.Context,
	req *connect.Request[metarrv1.AgentServiceSetLogLevelRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)
	slug := req.Msg.GetSlug()
	logLevel := req.Msg.GetLogLevel()

	if err := handlers.ValidateLogLevel(logLevel); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		if index := appconfig.FindAgentIndex(cfg, slug); index >= 0 {
			cfg.Agents[index].LogLevel = logLevel
		} else {
			cfg.Agents = append(cfg.Agents, &appconfig.AgentConfig{
				Slug:     slug,
				LogLevel: logLevel,
			})
		}
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

// validateMappings rejects an agent whose mappings could not work, returning
// the HTTP status to answer with. Moved verbatim from
// internal/server/handlers/agents.go (see agents_test.go, moved alongside
// it) — the plan's ported-not-wrapped pattern applies to pure helpers too.
func validateMappings(config *appconfig.Config, entry *appconfig.AgentConfig) (int, error) {
	seen := map[string]bool{}

	for _, mapping := range entry.Mappings {
		if appconfig.FindScanDirectoryIndex(config.DirectoryScanner, mapping.ScannerSlug) < 0 {
			return http.StatusBadRequest,
				fmt.Errorf("no scan directory with slug %q", mapping.ScannerSlug)
		}
		if seen[mapping.ScannerSlug] {
			return http.StatusBadRequest,
				fmt.Errorf("scan directory %q is mapped twice", mapping.ScannerSlug)
		}
		seen[mapping.ScannerSlug] = true

		// Two agents scanning one library would each overwrite the other's
		// records with its own view of the same files, so a scan directory
		// belongs to exactly one agent.
		if owner, mapped := appconfig.AgentForScanner(config, mapping.ScannerSlug); mapped && owner.Slug != entry.Slug {
			return http.StatusConflict,
				fmt.Errorf("scan directory %q is already mapped to agent %q", mapping.ScannerSlug, owner.Slug)
		}
	}

	return 0, nil
}
