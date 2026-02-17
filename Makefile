# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation

PROJECT_NAME             := upf
VERSION                  ?= $(shell cat ./VERSION 2>/dev/null || echo "dev")

# Extract minimum Go version from go.mod file
GOLANG_MINIMUM_VERSION   ?= $(shell awk '/^go / {print $$2}' go.mod 2>/dev/null || echo "1.25")

# Number of processors for parallel builds (Linux only)
NPROCS                   := $(shell nproc)

## Docker configuration
DOCKER_REGISTRY          ?=
DOCKER_REPOSITORY        ?=
DOCKER_TAG               ?= $(VERSION)
DOCKER_IMAGE_PREFIX      ?= 
DOCKER_IMAGENAME         := $(DOCKER_REGISTRY)$(DOCKER_REPOSITORY)$(DOCKER_IMAGE_PREFIX)$(PROJECT_NAME):$(DOCKER_TAG)
DOCKER_BUILDKIT          ?= 1
DOCKER_BUILD_ARGS        ?= --build-arg MAKEFLAGS=-j$(NPROCS)
DOCKER_PULL              ?= --pull

## Docker labels with better error handling
DOCKER_LABEL_VCS_URL     ?= $(shell git remote get-url origin 2>/dev/null || echo "unknown")
DOCKER_LABEL_VCS_REF     ?= $(shell \
	echo "$${GIT_COMMIT:-$${GITHUB_SHA:-$${CI_COMMIT_SHA:-$(shell \
		if git rev-parse --git-dir > /dev/null 2>&1; then \
			git rev-parse HEAD 2>/dev/null; \
		else \
			echo "unknown"; \
		fi \
	)}}}")
DOCKER_LABEL_BUILD_DATE  ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

DOCKER_TARGETS           ?= bess pfcpiface

# Golang grpc/protobuf generation
BESS_PB_DIR ?= pfcpiface
PTF_PB_DIR ?= ptf/lib

## Build configuration
BINARY_NAME              := $(PROJECT_NAME)
GO_PACKAGES              ?= ./...

## Directory configuration
BIN_DIR                  := bin
COVERAGE_DIR             := .coverage

## Go build configuration
GO_FILES                 := $(shell find . -name "*.go" ! -name "*_test.go" 2>/dev/null)
GO_FILES_ALL             := $(shell find . -name "*.go" 2>/dev/null)

## Tool versions (for reproducible builds)
GOLANGCI_LINT_VERSION    ?= latest

# Default target
.DEFAULT_GOAL := help

## Help target
help: ## Show this help message
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST) | sort

## Build targets
build: $(BIN_DIR)/$(BINARY_NAME) ## Build binary

all: build ## Build binary (alias for compatibility)

$(BIN_DIR)/$(BINARY_NAME): $(GO_FILES) | bin-dir
	@echo "Building $(BINARY_NAME)..."
	@CGO_ENABLED=0 go build -o $@ .

bin-dir: ## Create binary directory
	@mkdir -p $(BIN_DIR)

## Docker targets
docker-build: ## Build Docker image
	@go mod vendor
	for target in $(DOCKER_TARGETS); do \
		echo "Building Docker image: $(DOCKER_IMAGENAME)"; \
		DOCKER_CACHE_ARG=""; \
		if [ $(DOCKER_BUILDKIT) = 1 ]; then \
			DOCKER_CACHE_ARG="--cache-from ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}upf-$$target:${DOCKER_TAG}"; \
		fi; \
		DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_PULL) $(DOCKER_BUILD_ARGS) \
			--build-arg VERSION="$(VERSION)" \
			--build-arg VCS_URL="$(DOCKER_LABEL_VCS_URL)" \
			--build-arg VCS_REF="$(DOCKER_LABEL_VCS_REF)" \
			--build-arg BUILD_DATE="$(DOCKER_LABEL_BUILD_DATE)" \
			--tag ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}upf-$$target:${DOCKER_TAG} \
			. \
			|| exit 1; \
	done
	@rm -rf vendor

docker-push: ## Push Docker image to registry
	for target in $(DOCKER_TARGETS); do \
		echo "Pushing Docker image: $(DOCKER_IMAGENAME)"; \
		docker push ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}upf-$$target:${DOCKER_TAG} \
		|| exit 1; \
	done

docker-clean: ## Remove local Docker imagei
	@echo "Cleaning local Docker image..."
	for target in $(DOCKER_TARGETS); do \
		docker rmi ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}upf-$$target:${DOCKER_TAG} 2>/dev/null || true \
	done

# Change target to bess-build/pfcpiface to exctract src/obj/bins for performance analysis
output:
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_PULL) $(DOCKER_BUILD_ARGS) \
		--target artifacts \
		--output type=tar,dest=output.tar \
		.;
	rm -rf output && mkdir output && tar -xf output.tar -C output && rm -f output.tar

test-bess-integration-native:
	MODE=native DATAPATH=bess go test \
       -v \
       -race \
       -count=1 \
       -failfast \
       ./test/integration/...

pb:
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_PULL) $(DOCKER_BUILD_ARGS) \
		--target pb \
		--output output \
		.;
	cp -a output/bess_pb ${BESS_PB_DIR}

# Python grpc/protobuf generation
py-pb:
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_PULL) $(DOCKER_BUILD_ARGS) \
		--target ptf-pb \
		--output output \
		.;
	cp -a output/bess_pb/. ${PTF_PB_DIR}

## Testing targets
$(COVERAGE_DIR): ## Create coverage directory
	@mkdir -p $(COVERAGE_DIR)

test: $(COVERAGE_DIR) ## Run unit tests with coverage
	@echo "Running unit tests..."
	@docker run --rm \
		-v $(CURDIR):/$(PROJECT_NAME) \
		-w /$(PROJECT_NAME) \
		golang:$(GOLANG_MINIMUM_VERSION) \
		go test \
			-race \
			-failfast \
			-coverprofile=$(COVERAGE_DIR)/coverage-unit.txt \
			-covermode=atomic \
			-v \
			$(GO_PACKAGES)

test-local: $(COVERAGE_DIR) ## Run unit tests locally (without Docker)
	@echo "Running unit tests locally..."
	@go test \
		-race \
		-failfast \
		-coverprofile=$(COVERAGE_DIR)/coverage-unit.txt \
		-covermode=atomic \
		-v \
		$(GO_PACKAGES)

## Code quality targets
fmt: ## Format Go code
	@echo "Formatting Go code..."
	@go fmt ./...

lint: ## Run linter
	@echo "Running linter..."
	@docker run --rm \
		-v $(CURDIR):/app \
		-w /app \
		golangci/golangci-lint:$(GOLANGCI_LINT_VERSION) \
		golangci-lint run -v --config /app/.golangci.yml

lint-local: ## Run linter locally (without Docker)
	@echo "Running linter locally..."
	@golangci-lint run -v --config .golangci.yml

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
	@rm -rf $(BIN_DIR)
	@rm -rf $(COVERAGE_DIR)
	@rm -rf vendor
	@docker system prune -f --filter label=org.opencontainers.image.source="https://github.com/omec-project/$(PROJECT_NAME)" 2>/dev/null || true

print-version: ## Print current version
	@echo $(VERSION)

env: ## Print environment variables
	@echo "PROJECT_NAME=$(PROJECT_NAME)"
	@echo "VERSION=$(VERSION)"
	@echo "GOLANG_MINIMUM_VERSION=$(GOLANG_MINIMUM_VERSION)"
	@echo "BINARY_NAME=$(BINARY_NAME)"
	@echo "DOCKER_REGISTRY=$(DOCKER_REGISTRY)"
	@echo "DOCKER_REPOSITORY=$(DOCKER_REPOSITORY)"
	@echo "DOCKER_IMAGE_PREFIX=$(DOCKER_IMAGE_PREFIX)"
	@echo "DOCKER_TAG=$(DOCKER_TAG)"
	@echo "DOCKER_IMAGENAME=$(DOCKER_IMAGENAME)"
	@echo "DOCKER_LABEL_VCS_URL=$(DOCKER_LABEL_VCS_URL)"
	@echo "DOCKER_LABEL_VCS_REF=$(DOCKER_LABEL_VCS_REF)"
	@echo "NPROCS=$(NPROCS)"

## Phony targets
.PHONY: all \
        bin-dir \
        build \
        check \
        check-reuse \
        clean \
        docker-build \
        docker-clean \
        docker-push \
        env \
        fmt \
        help \
        lint \
        lint-local \
        print-version \
        test \
        test-local
