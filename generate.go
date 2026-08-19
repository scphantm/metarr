// Package generate holds repo-root go:generate directives that don't
// naturally belong to any other package. Run via `go generate ./...` (or
// `make build`, which runs it automatically before building).
package generate

// Regenerates docs/docs.go, docs/swagger.json, and docs/swagger.yaml from
// the swaggo annotations on the handlers in internal/handlers and the
// @title/@description block on cmd/metarr-server/main.go.
//go:generate go tool swag init -g cmd/metarr-server/main.go --output docs --parseInternal --parseDependency
