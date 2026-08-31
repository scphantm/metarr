package services

import (
	"context"
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
	"GetConfig":    {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateConfig": {Group: auth.GroupConfig},
	"GetTail":      {Group: auth.GroupConfig, ReadOnly: true},
	"StreamTail":   {Group: auth.GroupConfig, ReadOnly: true},
}

func (s *LoggingServer) GetConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.LoggingServiceGetConfigRequest],
) (*connect.Response[metarrv1.LoggingServiceGetConfigResponse], error) {
	appConfig := appconfig.Get()
	return connect.NewResponse(&metarrv1.LoggingServiceGetConfigResponse{
		Config: cloneMsg(appConfig.Logging),
	}), nil
}

func (s *LoggingServer) UpdateConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.LoggingServiceUpdateConfigRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	entry := req.Msg.GetConfig()
	if entry == nil {
		entry = &appconfig.LoggingConfig{}
	} else {
		entry = cloneMsg(entry)
	}
	if err := handlers.ValidateLogLevel(entry.ServerLevel); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		cfg.Logging = entry
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
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
