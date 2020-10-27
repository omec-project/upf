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

## Docker labels. Only set ref and commit date if committed
DOCKER_LABEL_VCS_URL     ?= $(shell git remote get-url $(shell git remote))
DOCKER_LABEL_VCS_REF     ?= $(shell git diff-index --quiet HEAD -- && git rev-parse HEAD || echo "unknown")
DOCKER_LABEL_COMMIT_DATE ?= $(shell git diff-index --quiet HEAD -- && git show -s --format=%cd --date=iso-strict HEAD || echo "unknown" )
DOCKER_LABEL_BUILD_DATE  ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

DOCKER_TARGETS           ?= bess cpiface pfcpiface ntf-pfcpiface

# https://docs.docker.com/engine/reference/commandline/build/#specifying-target-build-stage---target
docker-build:
	for target in $(DOCKER_TARGETS); do \
		DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build --pull $(DOCKER_BUILD_ARGS) \
			--target $$target \
			--tag ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}upf-epc-$$target:${DOCKER_TAG} \
			--build-arg org_label_schema_version="${VERSION}" \
			--build-arg org_label_schema_vcs_url="${DOCKER_LABEL_VCS_URL}" \
			--build-arg org_label_schema_vcs_ref="${DOCKER_LABEL_VCS_REF}" \
			--build-arg org_label_schema_build_date="${DOCKER_LABEL_BUILD_DATE}" \
			--build-arg org_opencord_vcs_commit_date="${DOCKER_LABEL_COMMIT_DATE}" \
			. \
			|| exit 1; \
	done

docker-push:
	for target in $(DOCKER_TARGETS); do \
		docker push ${DOCKER_REGISTRY}${DOCKER_REPOSITORY}upf-epc-$$target:${DOCKER_TAG}; \
	done

# Change target to bess-build/cpiface to exctract src/obj/bins for performance analysis
output:
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_BUILD_ARGS) \
		--target artifacts \
		--output output \
		.;

# Golang grpc/protobuf generation
BESS_PB_DIR ?= pfcpiface

pb:
	DOCKER_BUILDKIT=$(DOCKER_BUILDKIT) docker build $(DOCKER_BUILD_ARGS) \
		--target pb \
		--output output \
		.;
	cp -a output/bess_pb ${BESS_PB_DIR}

.PHONY: docker-build docker-push output pb
