package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"

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

// AgentServer implements metarrv1connect.AgentServiceHandler: the agent
// collection on AIP standard methods (docs/adr/0010). Operator fields are
// slug-addressed and persisted; the output-only presence fields (configured,
// online, identity, telemetry, reported_at) are merged in from the Redis
// presence records on every read and never written. Reads come from
// Registry.List (the same merge StreamPresence pushes); every write goes
// through AppConfigStore.MutateSync (docs/adr/0002).
type AgentServer struct {
	*handlers.Handlers
}

// AgentAuthPolicies is this service's method-name -> policy map. Every route
// is GroupConfig; the reads are read-only.
var AgentAuthPolicies = map[string]httpserver.RPCPolicy{
	"CreateAgent":    {Group: auth.GroupConfig},
	"GetAgent":       {Group: auth.GroupConfig, ReadOnly: true},
	"ListAgents":     {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateAgent":    {Group: auth.GroupConfig},
	"DeleteAgent":    {Group: auth.GroupConfig},
	"SetLogLevel":    {Group: auth.GroupConfig},
	"StreamPresence": {Group: auth.GroupConfig, ReadOnly: true},
}

// agentOrderFields maps the order_by paths ListAgents accepts to their
// comparators; any other path is InvalidArgument.
var agentOrderFields = map[string]func(a, b *metarrv1.Agent) int{
	"slug": func(a, b *metarrv1.Agent) int {
		return strings.Compare(a.GetSlug(), b.GetSlug())
	},
	"display_name": func(a, b *metarrv1.Agent) int {
		return strings.Compare(a.GetDisplayName(), b.GetDisplayName())
	},
}

// agentWritablePaths is the allow-list an UpdateAgent mask is checked
// against: slug is the addressing key and the presence fields are
// output-only, so a mask naming any of them is InvalidArgument.
var agentWritablePaths = map[string]bool{
	"display_name": true,
	"mappings":     true,
	"log_level":    true,
}

func (s *AgentServer) CreateAgent(
	ctx context.Context,
	req *connect.Request[metarrv1.CreateAgentRequest],
) (*connect.Response[metarrv1.Agent], error) {
	correlationID := correlation.FromContext(ctx)

	slug := req.Msg.GetAgentId()
	entry := sanitizeAgent(req.Msg.GetAgent())
	// AIP-133: a slug in the body must match agent_id or be empty.
	if bodySlug := entry.GetSlug(); bodySlug != "" && bodySlug != slug {
		return nil, connectError(http.StatusBadRequest,
			fmt.Errorf("agent.slug %q does not match agent_id %q", bodySlug, slug))
	}
	entry.Slug = slug
	if err := agentproto.ValidateSlug(entry.Slug); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	var stored *metarrv1.Agent
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		created, err := appendNewAgent(cfg, entry)
		stored = created
		return err
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}

func (s *AgentServer) GetAgent(
	ctx context.Context,
	req *connect.Request[metarrv1.GetAgentRequest],
) (*connect.Response[metarrv1.Agent], error) {
	agents, err := s.Agents.List(ctx, appconfig.Get())
	if err != nil {
		s.Logger.Error("failed to list agents", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to read agents"))
	}
	for _, agent := range agents {
		if agent.GetSlug() == req.Msg.GetSlug() {
			return connect.NewResponse(agent), nil
		}
	}
	return nil, connectError(http.StatusNotFound, errors.New("no agent with that slug"))
}

func (s *AgentServer) ListAgents(
	ctx context.Context,
	req *connect.Request[metarrv1.ListAgentsRequest],
) (*connect.Response[metarrv1.ListAgentsResponse], error) {
	if err := parseListFilter(req.Msg.GetFilter()); err != nil {
		return nil, aipConnectError(err)
	}

	agents, err := s.Agents.List(ctx, appconfig.Get())
	if err != nil {
		s.Logger.Error("failed to list agents", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to read agents"))
	}

	if err := orderBySlice(agents, req.Msg.GetOrderBy(), agentOrderFields); err != nil {
		return nil, aipConnectError(err)
	}

	page, nextPageToken, err := paginateSlice(agents, req.Msg.GetPageSize(), req.Msg.GetPageToken())
	if err != nil {
		return nil, aipConnectError(err)
	}
	return connect.NewResponse(&metarrv1.ListAgentsResponse{
		Agents:        page,
		NextPageToken: nextPageToken,
	}), nil
}

func (s *AgentServer) UpdateAgent(
	ctx context.Context,
	req *connect.Request[metarrv1.UpdateAgentRequest],
) (*connect.Response[metarrv1.Agent], error) {
	correlationID := correlation.FromContext(ctx)

	patch := req.Msg.GetAgent()
	if patch == nil {
		return nil, connectError(http.StatusBadRequest, errors.New("agent is required"))
	}
	slug := patch.GetSlug()
	if slug == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("agent.slug is required"))
	}
	for _, path := range req.Msg.GetUpdateMask().GetPaths() {
		if !agentWritablePaths[path] {
			return nil, connectError(http.StatusBadRequest,
				fmt.Errorf("%w: %q is not a writable agent field", errUnknownPath, path))
		}
	}

	var stored *metarrv1.Agent
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindAgentIndex(cfg, slug)
		if index == -1 {
			// allow_missing:true upgrades an Update on an unknown slug to a
			// Create — the mask is ignored and the whole resource is
			// validated as a Create (docs/adr/0010).
			if !req.Msg.GetAllowMissing() {
				return connectError(http.StatusNotFound, errors.New("no agent with that slug"))
			}
			created, err := appendNewAgent(cfg, sanitizeAgent(patch))
			stored = created
			return err
		}

		merged := cloneMsg(cfg.Agents[index])
		if err := applyUpdateMask(merged, patch, req.Msg.GetUpdateMask()); err != nil {
			return err
		}
		merged = sanitizeAgent(merged)
		// The mask cannot move the slug — it is the addressing key.
		merged.Slug = slug
		if status, err := validateMappings(cfg, merged); err != nil {
			return connectError(status, err)
		}
		cfg.Agents[index] = merged
		stored = configuredAgent(merged)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}

func (s *AgentServer) DeleteAgent(
	ctx context.Context,
	req *connect.Request[metarrv1.DeleteAgentRequest],
) (*connect.Response[emptypb.Empty], error) {
	correlationID := correlation.FromContext(ctx)
	slug := req.Msg.GetSlug()

	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindAgentIndex(cfg, slug)
		if index == -1 {
			return connectError(http.StatusNotFound, errors.New("no agent with that slug"))
		}
		cfg.Agents = append(cfg.Agents[:index], cfg.Agents[index+1:]...)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}

	// Drop the published projection immediately rather than waiting for the
	// config listener: a deleted agent should stop being able to read its
	// mapping now, not once the event has been processed.
	if err := s.Agents.Forget(ctx, slug); err != nil {
		s.Logger.Warn("could not remove the agent's published configuration", "agent", slug, "error", err)
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// SetLogLevel is a custom method (AIP-136): it sets one agent's log level
// without risking its mappings, and works for an agent that has only
// announced itself — a bare configuration entry is created for it. It returns
// the stored agent.
func (s *AgentServer) SetLogLevel(
	ctx context.Context,
	req *connect.Request[metarrv1.SetLogLevelRequest],
) (*connect.Response[metarrv1.Agent], error) {
	correlationID := correlation.FromContext(ctx)
	slug := req.Msg.GetSlug()
	logLevel := req.Msg.GetLogLevel()

	if err := handlers.ValidateLogLevel(logLevel); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	var stored *metarrv1.Agent
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		if index := appconfig.FindAgentIndex(cfg, slug); index >= 0 {
			cfg.Agents[index].LogLevel = logLevel
			stored = configuredAgent(cfg.Agents[index])
			return nil
		}
		entry := &metarrv1.Agent{Slug: slug, LogLevel: logLevel}
		cfg.Agents = append(cfg.Agents, entry)
		stored = configuredAgent(entry)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}

func (s *AgentServer) StreamPresence(
	ctx context.Context,
	req *connect.Request[metarrv1.StreamPresenceRequest],
	stream *connect.ServerStream[metarrv1.StreamPresenceResponse],
) error {
	ticker := time.NewTicker(agentPresenceStreamInterval)
	defer ticker.Stop()

	for {
		agents, err := s.Agents.List(ctx, appconfig.Get())
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.Logger.Error("failed to list agents", "error", err)
			return connectError(http.StatusInternalServerError, errors.New("failed to read agents"))
		}

		if err := stream.Send(&metarrv1.StreamPresenceResponse{Agents: agents}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// sanitizeAgent returns a copy of agent carrying only the operator-set,
// persisted fields. The output-only presence fields (configured, online,
// identity, telemetry, reported_at) are dropped — CreateAgent / UpdateAgent
// ignore whatever the request put there, and nothing about presence is ever
// written to the config document. A nil agent yields an empty one.
func sanitizeAgent(agent *metarrv1.Agent) *metarrv1.Agent {
	if agent == nil {
		return &metarrv1.Agent{}
	}
	clean := &metarrv1.Agent{
		Slug:        agent.GetSlug(),
		DisplayName: agent.GetDisplayName(),
		LogLevel:    agent.GetLogLevel(),
	}
	for _, mapping := range agent.GetMappings() {
		clean.Mappings = append(clean.Mappings, &metarrv1.AgentDirectoryMapping{
			ScannerSlug: mapping.GetScannerSlug(),
			AgentPath:   mapping.GetAgentPath(),
		})
	}
	return clean
}

// configuredAgent returns the write's stored resource: the sanitized operator
// fields with configured:true, since the agent now has a config entry.
// Presence stays zero — a subsequent Get / List / StreamPresence populates it
// from Redis.
func configuredAgent(agent *metarrv1.Agent) *metarrv1.Agent {
	clean := sanitizeAgent(agent)
	clean.Configured = true
	return clean
}

// appendNewAgent validates entry as a whole resource and adds it to cfg; an
// existing slug is AlreadyExists. It is the shared body of CreateAgent and
// the allow_missing branch of UpdateAgent. It returns the stored resource;
// the closures that call it run under the store lock.
func appendNewAgent(cfg *appconfig.Config, entry *metarrv1.Agent) (*metarrv1.Agent, error) {
	entry = sanitizeAgent(entry)
	if err := agentproto.ValidateSlug(entry.GetSlug()); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}
	if appconfig.FindAgentIndex(cfg, entry.GetSlug()) != -1 {
		return nil, connectError(http.StatusConflict,
			fmt.Errorf("an agent with slug %q already exists", entry.GetSlug()))
	}
	if status, err := validateMappings(cfg, entry); err != nil {
		return nil, connectError(status, err)
	}
	cfg.Agents = append(cfg.Agents, entry)
	return configuredAgent(entry), nil
}

// validateMappings rejects an agent whose mappings could not work, returning
// the HTTP status to answer with. Ported verbatim from
// internal/server/handlers/agents.go (see agents_test.go, moved alongside
// it) — the plan's ported-not-wrapped pattern applies to pure helpers too.
func validateMappings(config *appconfig.Config, entry *appconfig.Agent) (int, error) {
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
