.PHONY: build test cover lint vet check clean offline-resolve

BINARY  := strata
GOFLAGS := -v
FIXTURE := $(CURDIR)/bin/fixture

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

# offline-resolve proves a fresh clone can produce a lockfile with no AWS
# credentials and no network: it materializes the file:// fixture registry and
# resolves a profile through it end to end. CI runs the same sequence.
offline-resolve: build
	rm -rf $(FIXTURE)
	go run ./internal/testregistry/mkregistry $(FIXTURE)
	STRATA_REGISTRY_URL=file://$(FIXTURE)/registry \
		bin/$(BINARY) resolve $(FIXTURE)/profiles/offline-minimal.yaml -o $(FIXTURE)/offline.lock.yaml
	test -s $(FIXTURE)/offline.lock.yaml
	@echo "offline-resolve: lockfile at $(FIXTURE)/offline.lock.yaml"

clean:
	rm -rf bin/$(BINARY) coverage.out coverage.html $(FIXTURE)

.DEFAULT_GOAL := build
