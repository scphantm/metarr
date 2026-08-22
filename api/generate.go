package api

// Regenerates sonarr.gen.go from sonarr_openapi.yaml. Run via `go generate
// ./...` (or `make build`, which runs it automatically before building).
//go:generate go tool oapi-codegen -config sonarr.yaml sonarr_openapi.yaml
