# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation

PROJECT_NAME             := upf-epc
VERSION                  ?= $(shell cat ./VERSION)

# Note that we set the target platform of Docker images to native
# For a more portable image set CPU=haswell
CPU                      ?= native

# Enable Network Token Function support (see https://networktokens.org for more
# information)
ENABLE_NTF               ?= 0

## Docker related
DOCKER_REGISTRY          ?=
DOCKER_REPOSITORY        ?=
DOCKER_TAG               ?= ${VERSION}
DOCKER_IMAGENAME         := ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}${PROJECT_NAME}:${DOCKER_TAG}
DOCKER_BUILDKIT          ?= 1
DOCKER_BUILD_ARGS        ?= --build-arg MAKEFLAGS=-j$(shell nproc) --build-arg CPU
DOCKER_BUILD_ARGS        += --build-arg ENABLE_NTF=$(ENABLE_NTF)
DOCKER_PULL              ?= --pull

## Docker labels. Only set ref and commit date if committed
DOCKER_LABEL_VCS_URL     ?= $(shell git remote get-url $(shell git remote))
DOCKER_LABEL_VCS_REF     ?= $(shell git diff-index --quiet HEAD -- && git rev-parse HEAD || echo "unknown")
DOCKER_LABEL_COMMIT_DATE ?= $(shell git diff-index --quiet HEAD -- && git show -s --format=%cd --date=iso-strict HEAD || echo "unknown" )
DOCKER_LABEL_BUILD_DATE  ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

DOCKER_TARGETS           ?= bess pfcpiface

# Golang grpc/protobuf generation
BESS_PB_DIR ?= pfcpiface
PTF_PB_DIR ?= ptf/lib

# https://docs.docker.com/engine/reference/commandline/build/#specifying-target-build-stage---target
docker-build:
	for target in $(DOCKER_TARGETS); do \
		DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_PULL) $(DOCKER_BUILD_ARGS) \
			--target $$target \
			--tag ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}upf-epc-$$target:${DOCKER_TAG} \
			--label org.opencontainers.image.source="https://github.com/omec-project/upf-epc" \
			--label org.label.schema.version="${VERSION}" \
			--label org.label.schema.vcs.url="${DOCKER_LABEL_VCS_URL}" \
			--label org.label.schema.vcs.ref="${DOCKER_LABEL_VCS_REF}" \
			--label org.label.schema.build.date="${DOCKER_LABEL_BUILD_DATE}" \
			--label org.opencord.vcs.commit.date="${DOCKER_LABEL_COMMIT_DATE}" \
			. \
			|| exit 1; \
	done

docker-push:
	for target in $(DOCKER_TARGETS); do \
		docker push ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}upf-epc-$$target:${DOCKER_TAG}; \
	done

# Change target to bess-build/pfcpiface to exctract src/obj/bins for performance analysis
output:
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_PULL) $(DOCKER_BUILD_ARGS) \
		--target artifacts \
		--output type=tar,dest=output.tar \
		.;
	rm -rf output && mkdir output && tar -xf output.tar -C output && rm -f output.tar

test-up4-integration-docker:
	docker-compose -f test/integration/infra/docker-compose.yml rm -fsv
	COMPOSE_DOCKER_CLI_BUILD=1 DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker-compose -f test/integration/infra/docker-compose.yml build $(DOCKER_BUILD_ARGS)
	docker-compose -f test/integration/infra/docker-compose.yml up -d
	MODE=docker FASTPATH=up4 go test -v -count=1 -failfast ./test/integration/...
	docker-compose -f test/integration/infra/docker-compose.yml rm -fsv

test-bess-integration-native:
	MODE=native FASTPATH=bess go test -v -count=1 -failfast ./test/integration/...

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

.coverage:
	rm -rf $(CURDIR)/.coverage
	mkdir -p $(CURDIR)/.coverage

test: .coverage
	docker run --rm -v $(CURDIR):/upf-epc -w /upf-epc golang:latest \
		go test \
			-race \
			-failfast \
			-coverprofile=.coverage/coverage-unit.txt \
			-covermode=atomic \
			-v \
			./pfcpiface

p4-constants:
	$(info *** Generating go constants...)
	@docker run --rm -v $(CURDIR):/app -w /app \
		golang:latest go run ./scripts/go_gen_p4_const.go \
		-output internal/p4constants/p4constants.go -p4info conf/p4/bin/p4info.txt
	@docker run --rm -v $(CURDIR):/app -w /app \
		golang:latest gofmt -w internal/p4constants/p4constants.go

fmt:
	@go fmt ./...

golint:
	@docker run --rm -v $(CURDIR):/app -w /app/pfcpiface golangci/golangci-lint:latest golangci-lint run -v --config /app/.golangci.yml

check-reuse:
	@docker run --rm -v $(CURDIR):/upf-epc -w /upf-epc omecproject/reuse-verify:latest reuse lint

.PHONY: docker-build docker-push output pb fmt golint check-reuse test-up4-integration .coverage test
