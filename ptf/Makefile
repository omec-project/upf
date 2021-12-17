# SPDX-License-Identifier: Apache-2.0
# Copyright 2020-present Open Networking Foundation

# generates python protobuf files and builds ptf docker image
build: 
	cd .. && make py-pb
	docker build -t bess-upf-ptf .

# removes generated python protobuf files
clean:
	rm -v lib/*pb2*
	rm -rvf lib/ports/
