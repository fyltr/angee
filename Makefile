.PHONY: build build-operator build-cli test lint clean run-operator dev tools fmt vet check deps

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/fyltr/angee/cli.Version=$(VERSION)

# Locate `goimports` from GOPATH/bin even when it isn't on PATH (common after
# `go install` without PATH wiring). Falls back to whatever is on PATH.
GOIMPORTS ?= $(shell command -v goimports 2>/dev/null || echo "$$(go env GOPATH)/bin/goimports")

# ─── Build ────────────────────────────────────────────────

build: build-operator build-cli

build-operator:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/angee-operator ./cmd/operator

build-cli:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/angee ./cmd/angee

# Cross-compile for all platforms
build-all:
	$(MAKE) _build OS=linux   ARCH=amd64
	$(MAKE) _build OS=linux   ARCH=arm64
	$(MAKE) _build OS=darwin  ARCH=amd64
	$(MAKE) _build OS=darwin  ARCH=arm64
	$(MAKE) _build OS=windows ARCH=amd64

_build:
	GOOS=$(OS) GOARCH=$(ARCH) CGO_ENABLED=0 go build \
		-ldflags="$(LDFLAGS)" \
		-o dist/angee-$(OS)-$(ARCH) \
		./cmd/angee
	GOOS=$(OS) GOARCH=$(ARCH) CGO_ENABLED=0 go build \
		-ldflags="$(LDFLAGS)" \
		-o dist/angee-operator-$(OS)-$(ARCH) \
		./cmd/operator

# ─── Docker ───────────────────────────────────────────────

docker-operator:
	docker build -f Dockerfile.operator -t ghcr.io/fyltr/angee-operator:latest .

docker-cli:
	docker build -f Dockerfile.cli -t ghcr.io/fyltr/angee-cli:latest .

docker: docker-operator docker-cli

# ─── Dev ──────────────────────────────────────────────────

run-operator: build-operator
	ANGEE_ROOT=$(HOME)/.angee dist/angee-operator

dev: build-cli
	./dist/angee $(ARGS)

# ─── Test & lint ──────────────────────────────────────────

test:
	go test -v -race ./...

test-cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	@if [ -x "$(GOIMPORTS)" ]; then \
		$(GOIMPORTS) -w .; \
	else \
		echo "warning: goimports not found — run 'make tools' first"; \
	fi

# tools installs the developer-only binaries used by `make fmt` / `make lint`.
tools:
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1

vet:
	go vet ./...

check: fmt vet lint test

# ─── Deps ─────────────────────────────────────────────────

deps:
	go mod download
	go mod tidy

# ─── Clean ────────────────────────────────────────────────

clean:
	rm -rf dist/ coverage.out
