.PHONY: build test vet lint tidy release-dry clean docs-cli

BIN      := bin/linuxctl
PKG      := github.com/itunified-io/linuxctl
VERSION  ?= dev
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS  := -s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.Date=$(DATE)

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/linuxctl

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

release-dry:
	goreleaser release --snapshot --clean --skip=publish

clean:
	rm -rf bin dist

docs-cli:
	@mkdir -p docs/cli
	go run ./cmd/docgen docs/cli
