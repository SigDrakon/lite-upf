# Local development helper for lite-upf.
# The authoritative pipeline lives in .github/workflows/.

SHELL := /bin/bash

BINARY      := lite-upf
BUILD_DIR   := bin
MODULE      := github.com/SigDrakon/lite-upf
VERSION_PKG := $(MODULE)/internal/version

REPO_SLUG   := $(shell git config --get remote.origin.url 2>/dev/null | sed -E 's#.*github.com[:/]+##; s#\.git$$##' | tr '[:upper:]' '[:lower:]')
IMAGE       ?= ghcr.io/$(if $(REPO_SLUG),$(REPO_SLUG),sigdrakon/lite-upf)/lite-upf
IMAGE_TAG   ?= dev

VERSION     ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
BUILD_TIME  ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_HASH ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
COMMIT_TIME ?= $(shell git log -1 --pretty="@%at" 2>/dev/null | xargs -r date -u +"%Y-%m-%dT%H:%M:%SZ" -d 2>/dev/null || echo 1970-01-01T00:00:00Z)

LDFLAGS := -s -w \
	-X $(VERSION_PKG).Version=$(VERSION) \
	-X $(VERSION_PKG).BuildTime=$(BUILD_TIME) \
	-X $(VERSION_PKG).Commit=$(COMMIT_HASH)

GO_BUILD_ARGS := -trimpath -ldflags "$(LDFLAGS)"

.PHONY: all build run test test-race cover vet fmt fmt-check tidy lint verify docker-build docker-run docker-push clean

all: build

## build: compile the binary into ./bin/lite-upf
build:
	@echo "Building $(BINARY) ($(VERSION))..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(GO_BUILD_ARGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/lite-upf

## run: build and start the UPF with the example configuration
run: build
	$(BUILD_DIR)/$(BINARY) -config configs/config.yaml

## test: run the unit tests
test:
	go test ./...

## test-race: run the unit tests with the race detector
test-race:
	go test -race ./...

## cover: run the unit tests and report coverage
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

## vet: run go vet
vet:
	go vet ./...

## fmt: format the source tree
fmt:
	gofmt -s -w .

## fmt-check: fail if the tree is not formatted (this is what CI runs)
fmt-check:
	@out="$$(gofmt -s -l .)"; \
	if [ -n "$$out" ]; then \
		echo "The following files are not formatted:"; \
		echo "$$out"; \
		exit 1; \
	fi

## tidy: tidy go.mod / go.sum
tidy:
	go mod tidy

## lint: run golangci-lint (v2) if it is installed
lint:
	@command -v golangci-lint >/dev/null 2>&1 \
		&& golangci-lint run ./... \
		|| (echo "golangci-lint not installed; skipping. Install: https://golangci-lint.run/docs/welcome/install/" && exit 1)

## verify: everything CI runs before publishing an image
verify: fmt-check vet test lint

## docker-build: build the container image locally
docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--build-arg COMMIT_HASH=$(COMMIT_HASH) \
		--build-arg COMMIT_TIME=$(COMMIT_TIME) \
		-t $(IMAGE):$(IMAGE_TAG) \
		-t $(IMAGE):latest \
		.

## docker-run: run the image locally (needs NET_ADMIN / NET_RAW)
docker-run:
	docker run --rm -it \
		--cap-add=NET_ADMIN --cap-add=NET_RAW \
		--name lite-upf \
		-p 8805:8805/udp -p 2152:2152/udp \
		-v $(PWD)/configs:/lite-upf/config:ro \
		$(IMAGE):$(IMAGE_TAG)

## docker-push: push the image to the registry
docker-push:
	docker push $(IMAGE):$(IMAGE_TAG)

## clean: remove build artifacts
clean:
	rm -rf $(BUILD_DIR) coverage.out
