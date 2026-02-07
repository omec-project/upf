# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation
# SPDX-FileCopyrightText: 2026 Intel Corporation

PROJECT_NAME             := upf
VERSION                  ?= $(shell cat ./VERSION 2>/dev/null || echo "dev")
OSTYPE                   := $(shell uname -s)

# Extract Go version from go.mod file
GOLANG_VERSION           ?= $(shell awk '/^go / {print $$2}' go.mod 2>/dev/null || echo "1.21")

# Determine number of processors for parallel builds
ifeq ($(OSTYPE),Linux)
NPROCS                   := $(shell nproc)
else ifeq ($(OSTYPE),Darwin) # Assume Mac OS X
NPROCS                   := $(shell sysctl -n hw.physicalcpu)
else
NPROCS                   := 1
endif

# Build configuration
CPU                      ?= native
ENABLE_NTF               ?= 0

## Docker configuration
DOCKER_REGISTRY          ?=
DOCKER_REPOSITORY        ?=
DOCKER_TAG               ?= $(VERSION)
DOCKER_IMAGENAME         := $(DOCKER_REGISTRY)$(DOCKER_REPOSITORY)$(PROJECT_NAME):$(DOCKER_TAG)
DOCKER_BUILDKIT          ?= 1
DOCKER_BUILD_ARGS        ?= --build-arg MAKEFLAGS=-j$(NPROCS) --build-arg CPU=$(CPU)
DOCKER_BUILD_ARGS        += --build-arg ENABLE_NTF=$(ENABLE_NTF)
DOCKER_PULL              ?= --pull

## Docker labels with better error handling
DOCKER_LABEL_VCS_URL     ?= $(shell git remote get-url origin 2>/dev/null || echo "unknown")
DOCKER_LABEL_VCS_REF     ?= $(shell git diff-index --quiet HEAD -- 2>/dev/null && git rev-parse HEAD 2>/dev/null || echo "unknown")
DOCKER_LABEL_COMMIT_DATE ?= $(shell git diff-index --quiet HEAD -- 2>/dev/null && git show -s --format=%cd --date=iso-strict HEAD 2>/dev/null || echo "unknown")
DOCKER_LABEL_BUILD_DATE  ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

## Build targets
DOCKER_TARGETS           ?= bess pfcpiface
GO_PACKAGES              ?= ./pfcpiface ./cmd/...

## Directory configuration
BESS_PB_DIR              ?= pfcpiface
PTF_PB_DIR               ?= ptf/lib
COVERAGE_DIR             := .coverage
BUILD_OUTPUT_DIR         := build-output

## Tool versions (for reproducible builds)
GOLANGCI_LINT_VERSION    ?= latest

# Default target
.DEFAULT_GOAL := help

## Help target
help: ## Show this help message
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST) | sort

## Build targets
docker-build: ## Build Docker images for all targets
	@echo "Building Docker images for targets: $(DOCKER_TARGETS)"
	@for target in $(DOCKER_TARGETS); do \
		echo "Building $$target..."; \
		DOCKER_CACHE_ARG=""; \
		if [ "$(DOCKER_BUILDKIT)" = "1" ]; then \
			DOCKER_CACHE_ARG="--cache-from $(DOCKER_REGISTRY)$(DOCKER_REPOSITORY)$(PROJECT_NAME)-$$target:$(DOCKER_TAG)"; \
		fi; \
		DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_PULL) $(DOCKER_BUILD_ARGS) \
			--target $$target \
			$$DOCKER_CACHE_ARG \
			--tag $(DOCKER_REGISTRY)$(DOCKER_REPOSITORY)$(PROJECT_NAME)-$$target:$(DOCKER_TAG) \
			--label org.opencontainers.image.source="https://github.com/omec-project/$(PROJECT_NAME)" \
			--label org.opencontainers.image.version="$(VERSION)" \
			--label org.opencontainers.image.created="$(DOCKER_LABEL_BUILD_DATE)" \
			--label org.opencontainers.image.revision="$(DOCKER_LABEL_VCS_REF)" \
			--label org.opencontainers.image.url="$(DOCKER_LABEL_VCS_URL)" \
			--label org.label.schema.version="$(VERSION)" \
			--label org.label.schema.vcs.url="$(DOCKER_LABEL_VCS_URL)" \
			--label org.label.schema.vcs.ref="$(DOCKER_LABEL_VCS_REF)" \
			--label org.label.schema.build.date="$(DOCKER_LABEL_BUILD_DATE)" \
			--label org.opencord.vcs.commit.date="$(DOCKER_LABEL_COMMIT_DATE)" \
			. \
			|| exit 1; \
	done

docker-push: ## Push Docker images to registry
	@echo "Pushing Docker images for targets: $(DOCKER_TARGETS)"
	@for target in $(DOCKER_TARGETS); do \
		echo "Pushing $$target..."; \
		docker push $(DOCKER_REGISTRY)$(DOCKER_REPOSITORY)$(PROJECT_NAME)-$$target:$(DOCKER_TAG) || exit 1; \
	done

docker-clean: ## Remove local Docker images
	@echo "Cleaning local Docker images..."
	@for target in $(DOCKER_TARGETS); do \
		docker rmi $(DOCKER_REGISTRY)$(DOCKER_REPOSITORY)$(PROJECT_NAME)-$$target:$(DOCKER_TAG) 2>/dev/null || true; \
	done

## Development targets
$(BUILD_OUTPUT_DIR):
	@mkdir -p $(BUILD_OUTPUT_DIR)

output: $(BUILD_OUTPUT_DIR) ## Extract build artifacts
	@echo "Extracting build artifacts..."
	@DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_PULL) $(DOCKER_BUILD_ARGS) \
		--target artifacts \
		--output type=tar,dest=$(BUILD_OUTPUT_DIR)/output.tar \
		.
	@cd $(BUILD_OUTPUT_DIR) && tar -xf output.tar && rm -f output.tar

pb: $(BUILD_OUTPUT_DIR) ## Generate Go protobuf files
	@echo "Generating Go protobuf files..."
	@DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_PULL) $(DOCKER_BUILD_ARGS) \
		--target pb \
		--output $(BUILD_OUTPUT_DIR) \
		.
	@cp -a $(BUILD_OUTPUT_DIR)/bess_pb $(BESS_PB_DIR)

py-pb: $(BUILD_OUTPUT_DIR) ## Generate Python protobuf files
	@echo "Generating Python protobuf files..."
	@DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_PULL) $(DOCKER_BUILD_ARGS) \
		--target ptf-pb \
		--output $(BUILD_OUTPUT_DIR) \
		.
	@cp -a $(BUILD_OUTPUT_DIR)/bess_pb/. $(PTF_PB_DIR)

## Testing targets
$(COVERAGE_DIR):
	@mkdir -p $(COVERAGE_DIR)

test: $(COVERAGE_DIR) ## Run unit tests with coverage
	@echo "Running unit tests..."
	@docker run --rm \
		-v $(CURDIR):/$(PROJECT_NAME) \
		-w /$(PROJECT_NAME) \
		golang:$(GOLANG_VERSION) \
		go test \
			-race \
			-failfast \
			-coverprofile=$(COVERAGE_DIR)/coverage-unit.txt \
			-covermode=atomic \
			-v \
			$(GO_PACKAGES)

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@MODE=native DATAPATH=bess go test \
		-v \
		-race \
		-count=1 \
		-failfast \
		./test/integration/...

test-all: test test-integration ## Run all tests

## Code quality targets
fmt: ## Format Go code
	@echo "Formatting Go code..."
	@go fmt ./...

lint: ## Run linter
	@echo "Running linter..."
	@docker run --rm \
		-v $(CURDIR):/app \
		-w /app/pfcpiface \
		golangci/golangci-lint:$(GOLANGCI_LINT_VERSION) \
		golangci-lint run -v --config /app/.golangci.yml

check-reuse: ## Check REUSE compliance
	@echo "Checking REUSE compliance..."
	@docker run --rm \
		-v $(CURDIR):/$(PROJECT_NAME) \
		-w /$(PROJECT_NAME) \
		omecproject/reuse-verify:latest \
		reuse lint

check: fmt lint check-reuse ## Run all code quality checks

## Utility targets
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(COVERAGE_DIR) $(BUILD_OUTPUT_DIR)
	@docker system prune -f --filter label=org.opencontainers.image.source="https://github.com/omec-project/$(PROJECT_NAME)" 2>/dev/null || true

print-docker-targets: ## Print Docker build targets
	@echo $(DOCKER_TARGETS)

print-version: ## Print current version
	@echo $(VERSION)

env: ## Print environment variables
	@echo "PROJECT_NAME=$(PROJECT_NAME)"
	@echo "VERSION=$(VERSION)"
	@echo "DOCKER_REGISTRY=$(DOCKER_REGISTRY)"
	@echo "DOCKER_REPOSITORY=$(DOCKER_REPOSITORY)"
	@echo "DOCKER_TAG=$(DOCKER_TAG)"
	@echo "DOCKER_TARGETS=$(DOCKER_TARGETS)"
	@echo "NPROCS=$(NPROCS)"

## Phony targets
.PHONY: check \
        check-reuse \
        clean \
        docker-build \
        docker-clean \
        docker-push \
        env \
        fmt \
        help \
        lint \
        output \
        pb \
        print-docker-targets \
        print-version \
        py-pb \
        test \
        test-all \
        test-integration
