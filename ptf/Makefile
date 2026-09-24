# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation

.PHONY: build image dependency-smoke clean

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

# Removes generated Python protobuf files.
clean:
	rm -v lib/*pb2*
	rm -rvf lib/ports/
