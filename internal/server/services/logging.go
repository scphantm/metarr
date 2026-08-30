package services

import (
	"context"
	"encoding/json"
	"errors"
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
		Config: &metarrv1.LoggingConfig{
			ServerLevel: appConfig.Logging.ServerLevel,
			Sink:        appConfig.Logging.Sink,
			Endpoint:    appConfig.Logging.Endpoint,
			Stream:      appConfig.Logging.Stream,
		},
	}), nil
}

func (s *LoggingServer) UpdateConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.LoggingServiceUpdateConfigRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	entry := &appconfig.LoggingConfig{
		ServerLevel: req.Msg.GetConfig().GetServerLevel(),
		Sink:        req.Msg.GetConfig().GetSink(),
		Endpoint:    req.Msg.GetConfig().GetEndpoint(),
		Stream:      req.Msg.GetConfig().GetStream(),
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
	recordsJSON, err := json.Marshal(s.LogTail.Recent())
	if err != nil {
		s.Logger.Error("failed to encode log tail", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to encode log tail"))
	}
	return connect.NewResponse(&metarrv1.LoggingServiceGetTailResponse{RecordsJson: recordsJSON}), nil
}

func (s *LoggingServer) StreamTail(
	ctx context.Context,
	req *connect.Request[metarrv1.LoggingServiceStreamTailRequest],
	stream *connect.ServerStream[metarrv1.LoggingServiceStreamTailResponse],
) error {
	ticker := time.NewTicker(logTailStreamInterval)
	defer ticker.Stop()

	for {
		recordsJSON, err := json.Marshal(s.LogTail.Recent())
		if err != nil {
			s.Logger.Error("failed to encode log tail", "error", err)
			return connectError(http.StatusInternalServerError, errors.New("failed to encode log tail"))
		}

		if err := stream.Send(&metarrv1.LoggingServiceStreamTailResponse{RecordsJson: recordsJSON}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
