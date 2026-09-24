# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation

.PHONY: build image dependency-smoke trex-upstream-smoke clean

# Builds the PTF Docker image without generating BESS Python protobuf files.
image:
	docker build -t bess-upf-ptf .

# Generates Python protobuf files and builds the PTF Docker image for PTF tests.
build:
	cd .. && make py-pb
	$(MAKE) image

# Verifies the final image's resolver state and the PTF/TRex import boundary.
# This is intentionally separate from build while the legacy image has a known
# Scapy conflict; make it required in CI when #1220's migration lands.
dependency-smoke: image
	docker run --rm \
		-v "$(CURDIR):/upf-tests:ro" \
		-w /upf-tests \
		bess-upf-ptf \
		sh -ec 'PYTHONPATH="/upf-tests/lib:/upf-tests/tests/linerate:/upf-tests/tests/unary:$${PYTHONPATH}" exec python3 /upf-tests/dependency-smoke.py'

# Exercises the current upstream TRex client against the image's resolved
# Python dependencies. TRex dynamically adds its bundled Scapy to sys.path;
# remove that directory and its bootstrap entry in the disposable container
# before importing it.
trex-upstream-smoke:
	docker build \
		--build-arg TREX_REPO=cisco-system-traffic-generator/trex-core \
		--build-arg TREX_VER=3.06 \
		--target ptf-runtime-deps \
		-t bess-upf-ptf-trex-upstream \
		.
	docker run --rm \
		-v "$(CURDIR):/upf-tests:ro" \
		-w /upf-tests \
		bess-upf-ptf-trex-upstream \
		sh -ec 'sed -i "/'"'"'name'"'"': '"'"'scapy'"'"'/d" /trex_python/trex/__init__.py; rm -rf /external_libs/scapy-*; PYTHONPATH="/upf-tests/lib:/upf-tests/tests/linerate:/upf-tests/tests/unary:$${PYTHONPATH}" exec python3 /upf-tests/dependency-smoke.py'

# Removes generated Python protobuf files.
clean:
	rm -v lib/*pb2*
	rm -rvf lib/ports/
