.PHONY: build build-libghostty test tidy

build:
	go build ./cmd/council-ui

build-libghostty:
	go build -tags libghostty ./cmd/council-ui

test:
	go test ./...

tidy:
	go mod tidy
