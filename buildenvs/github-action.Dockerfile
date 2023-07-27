# SPDX-License-Identifier: BSD-3-Clause
# Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
# Licensed under the BSD-3-Clause License (the "License").
# You may not use this file except in compliance with the License.

ARG GO_VERSION=1.20.6
ARG GCC_SUFFIX=
ARG GCC_VERSION=12.2.0
ARG QEMU_VERSION=7.1.0-emulation-only
ARG REGISTRY=kraftkit.sh
ARG UK_ARCH=x86_64
ARG DEBIAN_VERSION=bullseye-20221114

FROM golang:${GO_VERSION}-bullseye AS github-action-build

COPY tools/github-action /go/src/github-action

WORKDIR /go/src/github-action

ENV GOROOT=/usr/local/go

# Compile the binary
RUN set -xe; \
    git config --global --add safe.directory /go/src/github-action; \
    go build -tags static -a -ldflags='-s -w' -ldflags '-extldflags "-static"' .; \
    mv github-action /;

FROM ${REGISTRY}/gcc:${GCC_VERSION}-x86_64${GCC_SUFFIX} AS gcc-x86_64
FROM ${REGISTRY}/qemu:${QEMU_VERSION} AS qemu
FROM debian:${DEBIAN_VERSION} AS github-action

ARG GCC_VERSION=12.2.0

COPY --from=github-action-build /github-action /github-action

COPY --from=gcc-x86_64 /bin/ /bin
COPY --from=gcc-x86_64 /lib/gcc/ /lib/gcc
COPY --from=gcc-x86_64 /include/ /include
COPY --from=gcc-x86_64 /x86_64-linux-gnu /x86_64-linux-gnu
COPY --from=gcc-x86_64 /libexec/gcc/x86_64-linux-gnu/${GCC_VERSION}/cc1 /libexec/gcc/x86_64-linux-gnu/${GCC_VERSION}/cc1
COPY --from=gcc-x86_64 /libexec/gcc/x86_64-linux-gnu/${GCC_VERSION}/collect2 /libexec/gcc/x86_64-linux-gnu/${GCC_VERSION}/collect2

COPY --from=qemu /bin/ /usr/local/bin
COPY --from=qemu /share/qemu/ /share/qemu
COPY --from=qemu /lib/x86_64-linux-gnu/ /lib/x86_64-linux-gnu

ARG GCC_PREFIX=x86_64-linux-gnu

# Link the GCC toolchain
RUN set -xe; \
    ln -s /bin/${GCC_PREFIX}-as         /bin/as; \
    ln -s /bin/${GCC_PREFIX}-ar         /bin/ar; \
    ln -s /bin/${GCC_PREFIX}-c++        /bin/c++; \
    ln -s /bin/${GCC_PREFIX}-c++filt    /bin/c++filt; \
    ln -s /bin/${GCC_PREFIX}-elfedit    /bin/elfedit; \
    ln -s /bin/${GCC_PREFIX}-gcc        /bin/cc; \
    ln -s /bin/${GCC_PREFIX}-gcc        /bin/gcc; \
    ln -s /bin/${GCC_PREFIX}-gcc-ar     /bin/gcc-ar; \
    ln -s /bin/${GCC_PREFIX}-gcc-nm     /bin/gcc-nm; \
    ln -s /bin/${GCC_PREFIX}-gcc-ranlib /bin/gcc-ranlib; \
    ln -s /bin/${GCC_PREFIX}-gccgo      /bin/gccgo; \
    ln -s /bin/${GCC_PREFIX}-gcov       /bin/gcov; \
    ln -s /bin/${GCC_PREFIX}-gcov-dump  /bin/gcov-dump; \
    ln -s /bin/${GCC_PREFIX}-gcov-tool  /bin/gcov-tool; \
    ln -s /bin/${GCC_PREFIX}-gprof      /bin/gprof; \
    ln -s /bin/${GCC_PREFIX}-ld         /bin/ld; \
    ln -s /bin/${GCC_PREFIX}-nm         /bin/nm; \
    ln -s /bin/${GCC_PREFIX}-objcopy    /bin/objcopy; \
    ln -s /bin/${GCC_PREFIX}-objdump    /bin/objdump; \
    ln -s /bin/${GCC_PREFIX}-ranlib     /bin/ranlib; \
    ln -s /bin/${GCC_PREFIX}-readelf    /bin/readelf; \
    ln -s /bin/${GCC_PREFIX}-size       /bin/size; \
    ln -s /bin/${GCC_PREFIX}-strings    /bin/strings; \
    ln -s /bin/${GCC_PREFIX}-strip      /bin/strip;

# Install unikraft dependencies
RUN set -xe; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
      make=4.3-4.1 \
      libncursesw5-dev=6.2+20201114-2+deb11u1 \
      libncursesw5=6.2+20201114-2+deb11u1 \
      libyaml-dev=0.2.2-1 \
      flex=2.6.4-8 \
      git=1:2.30.2-1+deb11u2 \
      wget=1.21-1+deb11u1 \
      patch=2.7.6-7 \
      gawk=1:5.1.0-1 \
      socat=1.7.4.1-3 \
      bison=2:3.7.5+dfsg-1 \
      bindgen=0.55.1-3+b1 \
      bzip2=1.0.8-4 \
      unzip=6.0-26+deb11u1 \
      uuid-runtime=2.36.1-8+deb11u1 \
      openssh-client=1:8.4p1-5+deb11u1 \
      autoconf=2.69-14 \
      xz-utils=5.2.5-2.1~deb11u1 \
      python3=3.9.2-3 \
      ca-certificates=20210119; \
    apt-get clean; \
    rm -Rf /var/cache/apt/*; \
    rm -Rf /var/lib/apt/lists/*;

WORKDIR /workspace

ENTRYPOINT [ "/github-action" ]