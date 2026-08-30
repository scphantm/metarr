package services

import (
	"context"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
)

// statsStreamInterval matches the interval wsbus.Hub.Register used for the
// stats.redis topic in cmd/metarr-server/main.go.
const statsStreamInterval = time.Second

// StatsServer implements metarrv1connect.StatsServiceHandler. Get is ported
// directly from internal/server/handlers/stats.go; Stream replaces the
// stats.redis wsbus topic, collecting on the same interval and reusing the
// exact same Collect call — only the delivery mechanism changed.
type StatsServer struct {
	*handlers.Handlers
}

// StatsAuthPolicies is this service's method-name -> policy map. Mirrors
// GET /api/stats/redis and the stats.redis topic both being GroupConfig.
var StatsAuthPolicies = map[string]httpserver.RPCPolicy{
	"Get":    {Group: auth.GroupConfig, ReadOnly: true},
	"Stream": {Group: auth.GroupConfig, ReadOnly: true},
}

func (s *StatsServer) Get(
	ctx context.Context,
	req *connect.Request[metarrv1.StatsServiceGetRequest],
) (*connect.Response[metarrv1.StatsServiceGetResponse], error) {
	snapshot, err := s.Stats.Collect(ctx)
	if err != nil {
		s.Logger.Error("failed to collect redis statistics", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to collect redis statistics"))
	}

	return connect.NewResponse(&metarrv1.StatsServiceGetResponse{Snapshot: snapshot}), nil
}

func (s *StatsServer) Stream(
	ctx context.Context,
	req *connect.Request[metarrv1.StatsServiceStreamRequest],
	stream *connect.ServerStream[metarrv1.StatsServiceStreamResponse],
) error {
	ticker := time.NewTicker(statsStreamInterval)
	defer ticker.Stop()

	for {
		snapshot, err := s.Stats.Collect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.Logger.Error("failed to collect redis statistics", "error", err)
			return connectError(http.StatusInternalServerError, errors.New("failed to collect redis statistics"))
		}

		if err := stream.Send(&metarrv1.StatsServiceStreamResponse{Snapshot: snapshot}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
