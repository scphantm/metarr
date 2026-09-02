package services

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// logTailStreamInterval matches the interval wsbus.Hub.Register used for
// the logging.tail topic in cmd/metarr-server/main.go.
const logTailStreamInterval = 2 * time.Second

// LoggingServer implements metarrv1connect.LoggingServiceHandler, ported
// directly from internal/server/handlers/logging.go's GetLoggingConfig,
// UpsertLoggingConfig and GetLogTail — same Mongo/buffer reads, only the
// transport changed. StreamTail replaces the logging.tail wsbus topic,
// reusing the exact same LogTail.Recent() call GetTail already makes.
type LoggingServer struct {
	*handlers.Handlers
}

// LoggingAuthPolicies is this service's method-name -> policy map. Mirrors
// the logging routes in router.go being GroupConfig.
var LoggingAuthPolicies = map[string]httpserver.RPCPolicy{
	"GetLoggingConfig":    {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateLoggingConfig": {Group: auth.GroupConfig},
	"GetTail":             {Group: auth.GroupConfig, ReadOnly: true},
	"StreamTail":          {Group: auth.GroupConfig, ReadOnly: true},
}

func (s *LoggingServer) GetLoggingConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.GetLoggingConfigRequest],
) (*connect.Response[metarrv1.GetLoggingConfigResponse], error) {
	// Live config is always Normalized, so the section already carries its
	// derived etag — the read just clones it out.
	return connect.NewResponse(&metarrv1.GetLoggingConfigResponse{
		Config: cloneMsg(appconfig.Get().Logging),
	}), nil
}

// UpdateLoggingConfig is an AIP-134 partial update: update_mask names the
// LoggingConfig fields to change, req.Config carries their new values, and the
// masked fields are merged onto the stored section under the config store's
// lock. req.Etag, when set, must match the stored section or the write is
// ABORTED (AIP-154). An empty mask or an unknown path returns InvalidArgument;
// the merged server_level is then validated so a bad level — or one blanked by
// a mask that names the field with no value — is rejected. The write returns
// an Operation the caller polls (docs/adr/0002).
func (s *LoggingServer) UpdateLoggingConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.UpdateLoggingConfigRequest],
) (*connect.Response[metarrv1.Operation], error) {
	correlationID := correlation.FromContext(ctx)

	patch := req.Msg.GetConfig()
	if patch == nil {
		return nil, connectError(http.StatusBadRequest, fmt.Errorf("logging config is required"))
	}

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		// cfg is Normalized, so cfg.Logging carries its current etag;
		// checkETag recomputes with that field cleared. MarshalStored strips
		// the derived etag before the section is persisted or fired.
		merged := cloneMsg(cfg.Logging)
		if merged == nil {
			merged = &metarrv1.LoggingConfig{}
		}
		if err := checkETag(merged, req.Msg.GetEtag()); err != nil {
			return err
		}
		if err := applyUpdateMask(merged, patch, req.Msg.GetUpdateMask()); err != nil {
			return err
		}
		if err := handlers.ValidateLogLevel(merged.ServerLevel); err != nil {
			return connectError(http.StatusBadRequest, err)
		}
		cfg.Logging = merged
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}

	return connect.NewResponse(beginConfigOperation(ctx, s.Operations, s.Logger, correlationID)), nil
}

func (s *LoggingServer) GetTail(
	ctx context.Context,
	req *connect.Request[metarrv1.LoggingServiceGetTailRequest],
) (*connect.Response[metarrv1.LoggingServiceGetTailResponse], error) {
	return connect.NewResponse(&metarrv1.LoggingServiceGetTailResponse{Records: s.LogTail.Recent()}), nil
}

func (s *LoggingServer) StreamTail(
	ctx context.Context,
	req *connect.Request[metarrv1.LoggingServiceStreamTailRequest],
	stream *connect.ServerStream[metarrv1.LoggingServiceStreamTailResponse],
) error {
	ticker := time.NewTicker(logTailStreamInterval)
	defer ticker.Stop()

	for {
		if err := stream.Send(&metarrv1.LoggingServiceStreamTailResponse{Records: s.LogTail.Recent()}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
