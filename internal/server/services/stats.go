package services

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
)

// StatsServer implements metarrv1connect.StatsServiceHandler. Both methods
// read the shared busstats.Sampler and never touch Redis inline: Get returns
// the last sampled snapshot, Stream fans out one frame per sampler pass.
type StatsServer struct {
	*handlers.Handlers
}

// StatsAuthPolicies is this service's method-name -> policy map. Both the
// first-paint read and the live stream sit behind GroupConfig, read-only.
var StatsAuthPolicies = map[string]httpserver.RPCPolicy{
	"Get":    {Group: auth.GroupConfig, ReadOnly: true},
	"Stream": {Group: auth.GroupConfig, ReadOnly: true},
}

func (s *StatsServer) Get(
	ctx context.Context,
	req *connect.Request[metarrv1.StatsServiceGetRequest],
) (*connect.Response[metarrv1.StatsServiceGetResponse], error) {
	snapshot := s.Stats.Get()
	if snapshot == nil {
		// Only reachable in the window before the sampler's first pass, which
		// Prime closes at startup. Answer honestly rather than with an OK
		// response carrying no snapshot.
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("bus snapshot not sampled yet"))
	}
	return connect.NewResponse(&metarrv1.StatsServiceGetResponse{Snapshot: snapshot}), nil
}

func (s *StatsServer) Stream(
	ctx context.Context,
	req *connect.Request[metarrv1.StatsServiceStreamRequest],
	stream *connect.ServerStream[metarrv1.StatsServiceStreamResponse],
) error {
	snapshots, unsubscribe := s.Stats.Subscribe()
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return nil
		case snapshot, ok := <-snapshots:
			if !ok {
				return nil
			}
			if err := stream.Send(&metarrv1.StatsServiceStreamResponse{Snapshot: snapshot}); err != nil {
				return err
			}
		}
	}
}
