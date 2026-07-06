VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS = -X github.com/abdullahranginwala/quibble/internal/cli.version=$(VERSION) \
          -X github.com/abdullahranginwala/quibble/internal/cli.commit=$(COMMIT)

.PHONY: build test gate install fmt

build:
	go build -ldflags "$(LDFLAGS)" -o bin/quibble ./cmd/quibble

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/quibble

test:
	go test ./... -race

fmt:
	gofmt -w .

gate:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "$$out"; echo "gofmt: files need formatting"; exit 1; fi
	go vet ./...
	go build ./...
	go test ./... -race
