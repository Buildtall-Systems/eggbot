.PHONY: help build dev test clean lint install tidy vendor

BINARY_NAME=eggbot
VERSION?=dev
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X 'github.com/buildtall-systems/eggbot/internal/cli.version=${VERSION}' -X 'github.com/buildtall-systems/eggbot/internal/cli.commit=${COMMIT}' -X 'github.com/buildtall-systems/eggbot/internal/cli.date=${DATE}' -X 'github.com/buildtall-systems/eggbot/internal/cli.builtBy=make'"

help:
	@echo "eggbot - Makefile commands"
	@echo ""
	@echo "  make build      - Build the binary"
	@echo "  make dev        - Run with live reload (requires air)"
	@echo "  make test       - Run tests"
	@echo "  make lint       - Run linter"
	@echo "  make tidy       - Tidy go modules"
	@echo "  make vendor     - Vendor dependencies for nix build"
	@echo "  make clean      - Clean build artifacts"
	@echo "  make install    - Install binary to GOPATH/bin"

build:
	go build ${LDFLAGS} -o bin/${BINARY_NAME} ./cmd/${BINARY_NAME}

dev:
	air

test:
	go test -v -race ./...

lint:
	golangci-lint run

tidy:
	go mod tidy

clean:
	rm -rf bin/ dist/ tmp/*.db

install: build
	go install ${LDFLAGS} ./cmd/${BINARY_NAME}

vendor:
	go mod tidy
	go mod vendor
