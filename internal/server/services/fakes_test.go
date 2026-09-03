package services

import (
	"context"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"

	"Metarr/internal/server/mongostore"
	"Metarr/internal/server/mongostore/versioned"
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

// fakeWorkflowStore is an in-memory WorkflowStore: one document id maps to an
// ordered slice of its versions (index 0 is version 1), appended to on every
// Save. It stamps the versioned.Envelope the way the real store does —
// minting a fresh version _id, bumping Version, and moving IsLatest onto the
// newest row — so WorkflowServer can be driven through the Connect handler
// seam with no MongoDB. No surface beyond the five WorkflowStore methods.
type fakeWorkflowStore struct {
	docs map[bson.ObjectID][]mongostore.Workflow
}

func newFakeWorkflowStore() *fakeWorkflowStore {
	return &fakeWorkflowStore{docs: map[bson.ObjectID][]mongostore.Workflow{}}
}

func (f *fakeWorkflowStore) Save(_ context.Context, documentID bson.ObjectID, w mongostore.Workflow) (mongostore.Workflow, error) {
	if documentID == bson.NilObjectID {
		documentID = bson.NewObjectID()
	}
	versions := f.docs[documentID]
	for i := range versions {
		versions[i].IsLatest = false
	}
	w.Envelope = versioned.Envelope{
		ID:         bson.NewObjectID(),
		DocumentID: documentID,
		Version:    len(versions) + 1,
		IsLatest:   true,
		CreatedAt:  time.Now().UTC(),
	}
	f.docs[documentID] = append(versions, w)
	return w, nil
}

func (f *fakeWorkflowStore) GetLatest(_ context.Context, documentID bson.ObjectID) (mongostore.Workflow, error) {
	versions := f.docs[documentID]
	if len(versions) == 0 {
		return mongostore.Workflow{}, versioned.ErrNotFound
	}
	return versions[len(versions)-1], nil
}

func (f *fakeWorkflowStore) GetVersion(_ context.Context, documentID bson.ObjectID, version int) (mongostore.Workflow, error) {
	versions := f.docs[documentID]
	if version < 1 || version > len(versions) {
		return mongostore.Workflow{}, versioned.ErrNotFound
	}
	return versions[version-1], nil
}

// ListVersions returns every version newest-first. An unknown id yields an
// empty slice and no error, matching versioned.Store.ListVersions — it never
// signals not-found.
func (f *fakeWorkflowStore) ListVersions(_ context.Context, documentID bson.ObjectID) ([]mongostore.Workflow, error) {
	versions := f.docs[documentID]
	out := make([]mongostore.Workflow, len(versions))
	for i, w := range versions {
		out[len(versions)-1-i] = w
	}
	return out, nil
}

// ListLatest mirrors versioned.Store.ListLatest closely enough for the seam
// tests: the latest row of each document, sorted by that row's _id
// descending, paged with an _id < cursor predicate and a fetch-one-extra
// hasMore probe.
func (f *fakeWorkflowStore) ListLatest(_ context.Context, filter versioned.LatestFilter) ([]mongostore.Workflow, string, bool, error) {
	latest := make([]mongostore.Workflow, 0, len(f.docs))
	for _, versions := range f.docs {
		latest = append(latest, versions[len(versions)-1])
	}
	sort.Slice(latest, func(i, j int) bool { return latest[i].ID.Hex() > latest[j].ID.Hex() })

	if filter.Cursor != bson.NilObjectID {
		kept := latest[:0:0]
		for _, w := range latest {
			if w.ID.Hex() < filter.Cursor.Hex() {
				kept = append(kept, w)
			}
		}
		latest = kept
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultWorkflowLimit
	}
	hasMore := int64(len(latest)) > limit
	if hasMore {
		latest = latest[:limit]
	}
	var nextCursor string
	if hasMore && len(latest) > 0 {
		nextCursor = latest[len(latest)-1].ID.Hex()
	}
	return latest, nextCursor, hasMore, nil
}
