// Package services implements the gRPC-Web (Connect) service layer that is
// replacing internal/server/handlers domain by domain. Each file here
// mirrors one handlers/*.go file one-for-one, moved rather than wrapped —
// same Mongo calls, same fireConfigUpdate/eventbus behavior, only the
// http.ResponseWriter/*http.Request signature becomes
// connect.Request/connect.Response.
package services

import (
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
