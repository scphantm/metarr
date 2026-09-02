// Package aip holds the transport-agnostic machinery the AIP-shaped
// configuration CRUD surface is built on (ADR-0010): resource-name parsing
// and formatting, FieldMask application over protobuf reflection, and the
// derived-field clearing a config-store mutation closure runs before the
// document is marshalled.
//
// Nothing here imports a transport package. Errors are returned as sentinels
// (ErrEmptyMask, ErrUnknownPath, ErrMalformedName) that the service layer
// maps to Connect codes; see internal/server/services/errors.go.
package aip
