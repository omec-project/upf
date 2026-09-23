# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation

.PHONY: build dependency-smoke clean

# Generates Python protobuf files and builds the PTF Docker image.
build:
	cd .. && make py-pb
	docker build -t bess-upf-ptf .

# Verifies the final image's resolver state and the PTF/TRex import boundary.
# This is intentionally separate from build while the legacy image has a known
# Scapy conflict; make it required in CI when #1220's migration lands.
dependency-smoke: build
	docker run --rm \
		-v "$(CURDIR):/upf-tests:ro" \
		-w /upf-tests \
		bess-upf-ptf \
		sh -ec 'PYTHONPATH="/upf-tests/lib:/upf-tests/tests/linerate:/upf-tests/tests/unary:$${PYTHONPATH}" exec python3 /upf-tests/dependency-smoke.py'

# Removes generated Python protobuf files.
clean:
	rm -v lib/*pb2*
	rm -rvf lib/ports/
