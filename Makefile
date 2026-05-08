.PHONY: build build-cli build-operator test fmt vet check clean

VERSION ?= dev
LDFLAGS := -s -w -X github.com/fyltr/angee/internal/cli.Version=$(VERSION)

build: build-cli build-operator

build-cli:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/angee ./cmd/angee

build-operator:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/angee-operator ./cmd/operator

test:
	go test -v -race ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

check: fmt vet test

clean:
	rm -rf dist coverage.out
