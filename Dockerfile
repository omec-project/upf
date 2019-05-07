# Multi-stage Dockerfile
# Stage bess-build: builds bess with its dependencies
FROM nefelinetworks/bess_build AS bess-build
ARG BESS_COMMIT=master
RUN apt-get update && apt-get install -y wget unzip ca-certificates git
RUN wget -qO bess.zip https://github.com/NetSys/bess/archive/${BESS_COMMIT}.zip && unzip bess.zip
WORKDIR bess-${BESS_COMMIT}
COPY core/modules/ core/modules/
RUN ./build.py bess && cp bin/bessd /bin
RUN mkdir -p /opt/bess && cp -r bessctl pybess /opt/bess

# Stage pip: compile psutil
FROM python:2.7-slim as pip
RUN apt-get update && apt-get install -y gcc
RUN pip install --no-cache-dir psutil

# Stage bess: creates the runtime image of BESS
FROM python:2.7-slim as bess
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libgraph-easy-perl \
        iproute2 \
        tcpdump && \
    rm -rf /var/lib/apt/lists/* && \
    pip install --no-cache-dir \
        grpcio \
        protobuf \
        pyroute2 \
        scapy
COPY --from=pip /usr/local/lib/python2.7/site-packages/psutil /usr/local/lib/python2.7/site-packages/psutil
COPY --from=bess-build /opt/bess /opt/bess
COPY --from=bess-build /bin/bessd /bin
WORKDIR /opt/bess/bessctl
COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
