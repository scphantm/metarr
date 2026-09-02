package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/handlers"
)

// fakeOperationStore is an in-memory handlers.OperationStore for the service
// seam — no Mongo. Create / Complete upsert by name the way OperationRepo does.
type fakeOperationStore struct {
	ops     map[string]*metarrv1.Operation
	getErr  error
	listErr error
}

func newFakeOperationStore() *fakeOperationStore {
	return &fakeOperationStore{ops: map[string]*metarrv1.Operation{}}
}

func (f *fakeOperationStore) Create(_ context.Context, name string) error {
	if _, ok := f.ops[name]; !ok {
		f.ops[name] = &metarrv1.Operation{Name: name}
	}
	return nil
}

func (f *fakeOperationStore) Complete(_ context.Context, name string, code int32, message string) error {
	op := &metarrv1.Operation{Name: name, Done: true}
	if code != 0 || message != "" {
		op.Result = &metarrv1.Operation_Error{Error: &metarrv1.OperationError{Code: code, Message: message}}
	}
	f.ops[name] = op
	return nil
}

func (f *fakeOperationStore) Get(_ context.Context, name string) (*metarrv1.Operation, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.ops[name], nil
}

func (f *fakeOperationStore) List(_ context.Context, done *bool, limit int64) ([]*metarrv1.Operation, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []*metarrv1.Operation
	for _, op := range f.ops {
		if done != nil && op.GetDone() != *done {
			continue
		}
		out = append(out, op)
		if limit > 0 && int64(len(out)) == limit {
			break
		}
	}
	return out, nil
}

func newTestOperationsServer(store handlers.OperationStore) *OperationsServer {
	return &OperationsServer{Handlers: &handlers.Handlers{
		Operations: store,
		Logger:     slog.Default(),
	}}
}

func TestGetOperation_ReturnsARecordedOperation(t *testing.T) {
	store := newFakeOperationStore()
	_ = store.Create(context.Background(), "operations/corr-1")
	_ = store.Complete(context.Background(), "operations/corr-1", 0, "")
	server := newTestOperationsServer(store)

	resp, err := server.GetOperation(context.Background(), connect.NewRequest(&metarrv1.GetOperationRequest{
		Name: "operations/corr-1",
	}))
	if err != nil {
		t.Fatalf("GetOperation: %v", err)
	}
	if !resp.Msg.GetDone() {
		t.Errorf("operation should be done: %+v", resp.Msg)
	}
	if resp.Msg.GetError() != nil {
		t.Errorf("a successful operation carried an error: %+v", resp.Msg.GetError())
	}
}

func TestGetOperation_UnknownNameIsNotFound(t *testing.T) {
	server := newTestOperationsServer(newFakeOperationStore())

	_, err := server.GetOperation(context.Background(), connect.NewRequest(&metarrv1.GetOperationRequest{
		Name: "operations/never-recorded",
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", connect.CodeOf(err))
	}
}

func TestGetOperation_MalformedNameIsInvalidArgument(t *testing.T) {
	server := newTestOperationsServer(newFakeOperationStore())

	for _, name := range []string{"", "corr-1", "operations/", "jobs/corr-1"} {
		_, err := server.GetOperation(context.Background(), connect.NewRequest(&metarrv1.GetOperationRequest{Name: name}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("name %q: code = %v, want InvalidArgument", name, connect.CodeOf(err))
		}
	}
}

func TestGetOperation_StoreFailureIsInternalAndOpaque(t *testing.T) {
	store := newFakeOperationStore()
	store.getErr = errors.New("mongo unreachable")
	server := newTestOperationsServer(store)

	_, err := server.GetOperation(context.Background(), connect.NewRequest(&metarrv1.GetOperationRequest{
		Name: "operations/corr-1",
	}))
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("code = %v, want Internal", connect.CodeOf(err))
	}
	if got := connect.CodeOf(err); got == connect.CodeInternal && err.Error() == "internal: mongo unreachable" {
		t.Errorf("internal error leaked its cause: %q", err.Error())
	}
}

func TestListOperations_FilterAndPageSize(t *testing.T) {
	store := newFakeOperationStore()
	for _, name := range []string{"operations/a", "operations/b", "operations/c"} {
		_ = store.Create(context.Background(), name)
	}
	_ = store.Complete(context.Background(), "operations/a", 0, "")
	server := newTestOperationsServer(store)

	t.Run("done filter", func(t *testing.T) {
		resp, err := server.ListOperations(context.Background(), connect.NewRequest(&metarrv1.ListOperationsRequest{
			Filter: "done = true",
		}))
		if err != nil {
			t.Fatalf("ListOperations: %v", err)
		}
		if len(resp.Msg.GetOperations()) != 1 || !resp.Msg.GetOperations()[0].GetDone() {
			t.Errorf("done=true filter returned %+v", resp.Msg.GetOperations())
		}
	})

	t.Run("page size clamps the result", func(t *testing.T) {
		resp, err := server.ListOperations(context.Background(), connect.NewRequest(&metarrv1.ListOperationsRequest{
			PageSize: 2,
		}))
		if err != nil {
			t.Fatalf("ListOperations: %v", err)
		}
		if len(resp.Msg.GetOperations()) != 2 {
			t.Errorf("page_size=2 returned %d operations", len(resp.Msg.GetOperations()))
		}
	})

	t.Run("bad filter is InvalidArgument", func(t *testing.T) {
		_, err := server.ListOperations(context.Background(), connect.NewRequest(&metarrv1.ListOperationsRequest{
			Filter: "name = x",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})
}
