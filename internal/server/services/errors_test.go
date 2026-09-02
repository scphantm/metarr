package services

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"testing"

	"connectrpc.com/connect"
)

// mutateConfigError is the sole error-mapping seam every config write funnels
// through. These pin the Connect codes the AIP reshape depends on: NotFound and
// AlreadyExists carried through from a mutation closure, and InvalidArgument
// synthesised from a field-mask sentinel that never learned about transport.
func TestMutateConfigError_CodeMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want connect.Code
	}{
		{"closure NotFound passes through", connectError(http.StatusNotFound, errors.New("no such slug")), connect.CodeNotFound},
		{"closure AlreadyExists passes through", connectError(http.StatusConflict, errors.New("slug taken")), connect.CodeAlreadyExists},
		{"closure InvalidArgument passes through", connectError(http.StatusBadRequest, errors.New("bad input")), connect.CodeInvalidArgument},
		{"empty mask maps to InvalidArgument", errEmptyMask, connect.CodeInvalidArgument},
		{"wrapped unknown path maps to InvalidArgument", fmt.Errorf("apply: %w", errUnknownPath), connect.CodeInvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mutateConfigError(slog.Default(), "corr-1", tc.err)
			if got := connect.CodeOf(err); got != tc.want {
				t.Fatalf("got code %v, want %v (err %v)", got, tc.want, err)
			}
		})
	}
}

func TestMutateConfigError_UnknownErrorIsInternal(t *testing.T) {
	_, err := mutateConfigError(slog.Default(), "corr-1", errors.New("mongo is on fire"))
	if got := connect.CodeOf(err); got != connect.CodeInternal {
		t.Fatalf("got code %v, want Internal", got)
	}
	if got := err.Error(); got == "mongo is on fire" {
		t.Fatalf("internal error leaked its cause to the client: %q", got)
	}
}

func TestAIPConnectError_IgnoresNonSentinel(t *testing.T) {
	if got := aipConnectError(errors.New("something else")); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}
