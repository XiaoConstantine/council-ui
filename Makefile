.PHONY: build test tidy

build:
	go build ./cmd/council-ui

test:
	go test ./...

tidy:
	go mod tidy
