# Multi-stage Dockerfile
# Stage bess-build: builds bess with its dependencies
FROM nefelinetworks/bess_build AS bess-build
RUN apt-get update && \
    apt-get -y install --no-install-recommends \
        build-essential \
        ca-certificates \
        libelf-dev \
        libnuma-dev \
        pkg-config \
        unzip \
        wget

ARG LINUX_VER=5.1.15
RUN wget -qO linux.tar.xz https://cdn.kernel.org/pub/linux/kernel/v5.x/linux-${LINUX_VER}.tar.xz
RUN mkdir linux && \
    tar -xf linux.tar.xz -C linux --strip-components 1 && \
    cp linux/include/uapi/linux/if_xdp.h /usr/include/linux && \
    cd linux/tools/lib/bpf/ && \
    make install_lib && \
    make install_headers && \
    ldconfig

# dpdk
ENV DPDK_VER=19.05
ENV DPDK_DIR="/dpdk"
ENV RTE_TARGET='x86_64-native-linuxapp-gcc'
RUN wget -qO dpdk.tar.xz https://fast.dpdk.org/rel/dpdk-${DPDK_VER}.tar.xz
RUN mkdir -p ${DPDK_DIR} && \
    tar -xf dpdk.tar.xz -C ${DPDK_DIR} --strip-components 1 && \
    cd ${DPDK_DIR} && \
    sed -ri 's,(IGB_UIO=).*,\1n,' config/common_linux* && \
    sed -ri 's,(KNI_KMOD=).*,\1n,' config/common_linux* && \
    sed -ri 's,(LIBRTE_BPF=).*,\1n,' config/common_base && \
    sed -ri 's,(AF_XDP=).*,\1y,' config/common_base && \
    make config T=x86_64-native-linuxapp-gcc && \
    make -j $CPUS EXTRA_CFLAGS="-g -w -fPIC"

# Workaround for compiler error on including DPDK in bess
WORKDIR ${DPDK_DIR}
COPY patches/dpdk patches
RUN cat patches/* | patch -p1 && \
    make -j $CPUS EXTRA_CFLAGS="-g -w -fPIC"

WORKDIR /
ARG BESS_COMMIT=master
RUN apt-get update && apt-get install -y wget unzip ca-certificates git
RUN wget -qO bess.zip https://github.com/NetSys/bess/archive/${BESS_COMMIT}.zip && unzip bess.zip
WORKDIR bess-${BESS_COMMIT}
COPY core/modules/ core/modules/
COPY protobuf/ protobuf/
RUN cp -a ${DPDK_DIR} deps/dpdk-17.11
COPY patches/bess patches
RUN cat patches/* | patch -p1
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
        iptables \
	iputils-ping \
        procps \
        tcpdump && \
    rm -rf /var/lib/apt/lists/* && \
    pip install --no-cache-dir \
        flask \
        grpcio \
        iptools \
        protobuf \
        pyroute2 \
        https://github.com/secdev/scapy/archive/b65e795c62accd383e1bb6b17cd9f7a9143ae117.zip
# Workaround for mismatch in GLIBC version in
# nefelinetworks/bess_build (ubuntu:bionic) and python:2.7-slim (debian:stretch)
# Should go away one python moves to buster
RUN echo "deb http://http.us.debian.org/debian testing main non-free contrib" > /etc/apt/sources.list.d/testing.list && \
    apt-get update && \
    apt-get -t testing -y install --no-install-recommends \
        libc6 && \
    rm -rf /var/lib/apt/lists/*
COPY --from=pip /usr/local/lib/python2.7/site-packages/psutil /usr/local/lib/python2.7/site-packages/psutil
COPY --from=bess-build /opt/bess /opt/bess
COPY --from=bess-build /bin/bessd /bin
RUN ln -s /opt/bess/bessctl/bessctl /bin
ENV PYTHONPATH="/opt/bess"
WORKDIR /opt/bess/bessctl
ENTRYPOINT ["bessd", "-f"]
