.PHONY: build test cover lint vet check clean

BINARY  := strata
GOFLAGS := -v

build:
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

cover: test
	go tool cover -html=coverage.out

lint:
	golangci-lint run ./...

vet:
	go vet ./...

check: vet lint test

clean:
	rm -f bin/$(BINARY) coverage.out coverage.html

.DEFAULT_GOAL := build
