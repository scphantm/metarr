.PHONY: generate build run test tidy

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
