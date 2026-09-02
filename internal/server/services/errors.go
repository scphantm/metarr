// Package services implements the gRPC-Web (Connect) service layer that is
// replacing internal/server/handlers domain by domain. Each file here
// mirrors one handlers/*.go file one-for-one, moved rather than wrapped —
// same Mongo calls, same fireConfigUpdate/eventbus behavior, only the
// http.ResponseWriter/*http.Request signature becomes
// connect.Request/connect.Response.
package services

import (
	"errors"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/shared/aip"
)

// connectError converts an HTTP status code (the vocabulary every existing
// handlers/*.go file already uses via http.Error) into the equivalent
// Connect/gRPC error. Existing pure validators that already return
// (http.Status*, error) — e.g. handlers/agents.go's validateMappings, see
// its test — need no changes at all: only the call site converts the
// returned status through this helper.
func connectError(status int, err error) error {
	return connect.NewError(codeForStatus(status), err)
}

// mutateConfigError turns an error returned by AppConfigStore.Mutate into
// the response pair every config-mutating RPC returns. A mutation closure's
// own rejection is already Connect-shaped (built with connectError, the
// same way these methods built it before the config store existed) and
// passes through unchanged; anything else — the store's own read or event
// failing — is logged and reported as a generic 500, matching what every
// one of these methods already did for that case.
func mutateConfigError(logger *slog.Logger, correlationID string, err error) (*connect.Response[metarrv1.AcceptedResponse], error) {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return nil, err
	}
	if mapped := aipConnectError(err); mapped != nil {
		return nil, mapped
	}
	logger.Error("failed to queue config update", "correlation_id", correlationID, "error", err)
	return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
}

// aipConnectError maps the transport-agnostic sentinels the aip package
// returns — an empty or malformed update_mask, a resource name that does not
// match its collection's pattern — to the Connect code the config API
// answers with (always InvalidArgument today). It returns nil for anything
// that is not an aip sentinel, so a caller can fall through to its existing
// handling. Read-path methods (Get / List) that call an aip helper directly
// wrap their error with this; write-path methods get the same mapping for
// free through mutateConfigError.
//
// AlreadyExists is produced the same way NotFound always has been — a
// mutation closure returning connectError(http.StatusConflict, …), which
// mutateConfigError passes straight through — so it needs no entry here.
func aipConnectError(err error) error {
	switch {
	case errors.Is(err, aip.ErrEmptyMask),
		errors.Is(err, aip.ErrUnknownPath),
		errors.Is(err, aip.ErrMalformedName):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return nil
	}
}

func codeForStatus(status int) connect.Code {
	switch status {
	case http.StatusBadRequest:
		return connect.CodeInvalidArgument
	case http.StatusUnauthorized:
		return connect.CodeUnauthenticated
	case http.StatusForbidden:
		return connect.CodePermissionDenied
	case http.StatusNotFound:
		return connect.CodeNotFound
	case http.StatusConflict:
		return connect.CodeAlreadyExists
	case http.StatusUnprocessableEntity:
		return connect.CodeFailedPrecondition
	case http.StatusTooManyRequests:
		return connect.CodeResourceExhausted
	case http.StatusBadGateway:
		return connect.CodeUnavailable
	case http.StatusGatewayTimeout:
		return connect.CodeDeadlineExceeded
	default:
		return connect.CodeInternal
	}
}
