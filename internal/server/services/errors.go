// Package services is the gRPC-Web (Connect) service layer. Each config
// service reads from live config and writes through the one synchronous path
// — appconfigstore.Store.MutateSync: persist under the store lock, propagate
// in-process, return the stored resource (docs/adr/0002 / docs/adr/0010).
// The non-config services (stats, tasks, workflows, …) keep their own
// direct Mongo or Redis calls.
package services

import (
	"errors"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
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

// mutateConfigErr maps an error returned by AppConfigStore.MutateSync to the
// Connect error a config-mutating RPC surfaces. A mutation closure's own
// rejection is already Connect-shaped (built with connectError, the same way
// these methods built it before the config store existed) and passes through
// unchanged; an aip sentinel (bad mask) is mapped; anything else — the
// store's own read or persist failing — is logged and reported as a generic
// 500, matching what every one of these methods already did for that case.
func mutateConfigErr(logger *slog.Logger, correlationID string, err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return err
	}
	if mapped := aipConnectError(err); mapped != nil {
		return mapped
	}
	logger.Error("failed to write config update", "correlation_id", correlationID, "error", err)
	return connectError(http.StatusInternalServerError, errors.New("failed to write config update"))
}

// aipConnectError maps the transport-agnostic AIP sentinels the services
// package raises to the Connect code the config API answers with:
//   - an empty or bad update_mask, a bad order_by, a bad page_token
//     (errEmptyMask / errUnknownPath / errBadPageToken) → InvalidArgument
//   - a filter expression (errFilterUnsupported) → Unimplemented (AIP-160
//     translation is deferred)
//
// It returns nil for anything that is not one of them, so a caller can fall
// through to its existing handling. Read-path methods (Get / List) that call
// an AIP helper directly wrap their error with this; write-path methods get
// the same mapping for free through mutateConfigErr.
//
// AlreadyExists is produced the same way NotFound always has been — a
// mutation closure returning connectError(http.StatusConflict, …), which
// mutateConfigErr passes straight through — so it needs no entry here.
func aipConnectError(err error) error {
	switch {
	case errors.Is(err, errEmptyMask),
		errors.Is(err, errUnknownPath),
		errors.Is(err, errBadPageToken):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, errFilterUnsupported):
		return connect.NewError(connect.CodeUnimplemented, err)
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
