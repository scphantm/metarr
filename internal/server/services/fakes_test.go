package services

import (
	"context"

	"google.golang.org/protobuf/proto"

	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/eventbus"
)

// fakeConfigBackend satisfies appconfigstore's Get/Upsert/Fire dependencies,
// persisting whatever a Fire or Upsert call carries so the next Get sees it
// — enough to drive a real ConfigServer end to end with no MongoDB or
// Redis. The config types are proto messages, so the document is held and
// handed out as a clone rather than aliased across the seam.
type fakeConfigBackend struct {
	cfg   *appconfig.Config
	fired []*eventbus.Event
}

func (f *fakeConfigBackend) Get(_ context.Context) (*appconfig.Config, error) {
	if f.cfg == nil {
		return &appconfig.Config{}, nil
	}
	return proto.Clone(f.cfg).(*appconfig.Config), nil
}

// Publish mirrors *eventbus.Bus: it is handed name, correlation ID and
// payload, and the Bus stamps Source and Timestamp itself (Timestamp is left
// zero here — it is the Bus's to stamp, not the caller's).
func (f *fakeConfigBackend) Publish(_ context.Context, _ eventbus.StreamTopic, name, correlationID string, payload []byte) error {
	cfg, err := appconfig.UnmarshalStored(payload)
	if err != nil {
		return err
	}
	f.cfg = cfg
	f.fired = append(f.fired, &eventbus.Event{
		Name:          name,
		Source:        eventbus.SourceServer,
		CorrelationId: correlationID,
		Payload:       payload,
	})
	return nil
}

func (f *fakeConfigBackend) Upsert(_ context.Context, cfg *appconfig.Config) error {
	f.cfg = proto.Clone(cfg).(*appconfig.Config)
	return nil
}

// liveConfigPropagator is the in-process propagation MutateSync runs after a
// successful write, reduced to the one step a handler round-trip test cares
// about: making the freshly written document the live config a subsequent Get
// reads. Pair it with withLiveConfig so the singleton is restored afterwards.
type liveConfigPropagator struct{}

func (liveConfigPropagator) PropagateInProcess(_ context.Context, cfg *appconfig.Config) error {
	appconfig.Set(appconfig.Normalize(cfg))
	return nil
}
