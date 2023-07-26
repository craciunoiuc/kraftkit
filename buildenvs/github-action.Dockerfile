# SPDX-License-Identifier: BSD-3-Clause
# Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
# Licensed under the BSD-3-Clause License (the "License").
# You may not use this file except in compliance with the License.

ARG GO_VERSION=1.20.6

FROM golang:${GO_VERSION}-bullseye AS github-action

# Install build dependencies
RUN set -xe; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
      build-essential \
      cmake \
      libssh2-1-dev \
      libssl-dev \
      make \
      pkg-config \
      git; \
    apt-get clean; \
    go install mvdan.cc/gofumpt@v0.4.0; \
    git config --global --add safe.directory /go/src/github-action;

WORKDIR /go/src/github-action

COPY tools/github-action /go/src/github-action

# Compile the binary
RUN set -xe; \
    go build -tags static -a -ldflags='-s -w' -ldflags '-extldflags "-static"' .; \
    mv github-action /; \
    /github-action -h;

ENV GOROOT=/usr/local/go
ENV PAGER=cat

ENTRYPOINT [ "/github-action" ]