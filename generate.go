// Package generate holds repo-root go:generate directives that don't
// naturally belong to any other package. Run via `go generate ./...` (or
// `make build`, which runs it automatically before building).
package generate

// Regenerates api/docs.go, api/swagger.json, and api/swagger.yaml from
// the swaggo annotations on the handlers in internal/handlers and the
// @title/@description block on cmd/metarr-server/main.go.
//go:generate go tool swag init -g cmd/metarr-server/main.go --output api --parseInternal --parseDependency

// Regenerates the gRPC-Web surface from proto/: Go server stubs into
// internal/genproto/ (protoc-gen-go, protoc-gen-connect-go) and TypeScript
// client stubs into ui/src/gen/ (BSR-hosted protoc-gen-es/connect-es, via
// buf.gen.yaml's remote plugins). Yes, this one directive writes into ui/ —
// there's deliberately no second, UI-side generate step, so there's only one
// place "did I regenerate" can be forgotten rather than two.
//go:generate go tool buf generate
