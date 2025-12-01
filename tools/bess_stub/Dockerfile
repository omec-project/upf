# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2019-present Intel Corporation
# Copyright (c) 2024-present The UPF contributors

FROM python:3.11-slim
WORKDIR /app

# Copy only what we need from the repo so the image build is quick.
COPY pfcpiface /app/pfcpiface
COPY tools/bess_stub/server.py /app/server.py

RUN pip install --no-cache-dir grpcio==1.56.2 protobuf==4.25.8

EXPOSE 10514

CMD ["python", "server.py"]
