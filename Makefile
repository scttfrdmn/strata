.PHONY: build test cover lint vet vuln check clean offline-resolve

BINARY  := strata
GOFLAGS := -v
FIXTURE := $(CURDIR)/bin/fixture

# The linter version CI pins (.github/workflows/ci.yml). `make lint` warns when
# the golangci-lint on PATH differs, since a local pass under a different version
# is not the same statement as CI passing (#75).
GOLANGCI_VERSION := v2.11.3
# govulncheck version, pinned so the tool cannot change under us (#75).
GOVULNCHECK_VERSION := v1.1.4

build:
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

cover: test
	go tool cover -html=coverage.out

lint:
	@golangci-lint version 2>/dev/null | grep -q '$(GOLANGCI_VERSION:v%=%)' || \
	  echo "warning: golangci-lint on PATH differs from CI's pinned $(GOLANGCI_VERSION); findings may differ (#75)"
	golangci-lint run ./...

vet:
	go vet ./...

# vuln scans for known vulnerabilities the code actually reaches. Needs network
# (the vuln database is fetched at run time); CI runs the same as its own job.
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

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
