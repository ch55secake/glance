GO ?= go
GOLANGCI_LINT_VERSION ?= v2.13.1

.PHONY: all build run fmt test test-race vet lint check

all: build

build:
	mkdir -p bin
	$(GO) build -o bin/glance ./cmd/glance

run:
	$(GO) run ./cmd/glance

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --timeout=5m

check: fmt test vet build
