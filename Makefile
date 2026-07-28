.PHONY: generate build run test tidy install-cli

generate:
	go generate ./...

build: generate
	go build ./...

run: generate
	go run ./cmd/api

test: generate
	go test ./...

tidy:
	go mod tidy

install-cli:
	go install ./cmd/metarrctl
