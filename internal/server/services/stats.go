package services

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/shared/eventbus"
)

// StatsServer implements metarrv1connect.StatsServiceHandler. Get and Stream
// read the shared busstats.Sampler and never touch Redis inline: Get returns
// the last sampled snapshot, Stream fans out one frame per sampler pass.
// Purge is the one method that writes to Redis — an operator clearing a
// jammed durable stream.
type StatsServer struct {
	*handlers.Handlers
}

// StatsAuthPolicies is this service's method-name -> policy map. The
// first-paint read and the live stream sit behind GroupConfig, read-only;
// Purge sits behind the same group but is a write, so it is not read-only —
// a read-only caller is refused it.
var StatsAuthPolicies = map[string]httpserver.RPCPolicy{
	"Get":    {Group: auth.GroupConfig, ReadOnly: true},
	"Stream": {Group: auth.GroupConfig, ReadOnly: true},
	"Purge":  {Group: auth.GroupConfig},
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

// Purge clears a jammed durable stream: an approximate trim of every current
// entry, then a fast-forward of each consumer group to the stream's tail. It
// clears one stream by name, or every discovered durable stream when the
// request sets all. The stream key and its groups stay in place — see
// docs/adr/0007. Each purged stream is recorded at warn level with the
// acting identity, the stream, and the dropped-entry count: an incident tool
// leaves an audit trail.
func (s *StatsServer) Purge(
	ctx context.Context,
	req *connect.Request[metarrv1.StatsServicePurgeRequest],
) (*connect.Response[metarrv1.StatsServicePurgeResponse], error) {
	stream := req.Msg.GetStream()
	all := req.Msg.GetAll()
	if all == (stream != "") {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("purge requires exactly one of stream or all"))
	}
	if s.Redis == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("event bus is not connected"))
	}

	var purges []eventbus.StreamPurge
	var err error
	if all {
		purges, err = eventbus.PurgeAllStreams(ctx, s.Redis)
	} else {
		var one eventbus.StreamPurge
		one, err = eventbus.PurgeStream(ctx, s.Redis, stream)
		if err == nil {
			purges = []eventbus.StreamPurge{one}
		}
	}

	// Log every stream that was actually cleared before surfacing any
	// failure, so the audit trail survives a partial purge-all.
	actor := purgeActor(ctx)
	results := make([]*metarrv1.StreamPurgeResult, 0, len(purges))
	for _, purge := range purges {
		s.Logger.Warn("bus stream purged",
			"actor", actor, "stream", purge.Stream, "dropped", purge.Dropped)
		results = append(results, &metarrv1.StreamPurgeResult{
			Stream:              purge.Stream,
			Dropped:             purge.Dropped,
			GroupsFastForwarded: purge.GroupsFastForwarded,
		})
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&metarrv1.StatsServicePurgeResponse{Results: results}), nil
}

// purgeActor names who requested the purge for the audit log. The interceptor
// resolves the caller's API key to a role and stores it on the context;
// GroupConfig is admin-only today, so this is "admin" in practice, but read
// it rather than assume it.
func purgeActor(ctx context.Context) string {
	if role, ok := auth.RoleFromContext(ctx); ok {
		return string(role)
	}
	return "unknown"
}
