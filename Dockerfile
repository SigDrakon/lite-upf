# syntax=docker/dockerfile:1.7
#
# lite-upf container image.
#
# Build:
#   docker build -t lite-upf:local .
#
# The version metadata is injected via build args so that `lite-upf -version`
# reports which commit the image was built from.

ARG GO_VERSION=1.26.2
ARG ALPINE_VERSION=3.22

# ---------------------------------------------------------------------------
# Stage 1: compile the statically linked binary
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG BUILD_TIME=1970-01-01T00:00:00Z
ARG COMMIT_HASH=unknown
ARG COMMIT_TIME=1970-01-01T00:00:00Z

WORKDIR /src

# Dependencies first so the layer is cached across source-only changes.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

# CGO_ENABLED=0 produces a static binary that runs on a minimal base image.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
      -trimpath \
      -ldflags "-s -w \
        -X github.com/SigDrakon/lite-upf/internal/version.Version=${VERSION} \
        -X github.com/SigDrakon/lite-upf/internal/version.BuildTime=${BUILD_TIME} \
        -X github.com/SigDrakon/lite-upf/internal/version.Commit=${COMMIT_HASH}" \
      -o /out/lite-upf ./cmd/lite-upf

# ---------------------------------------------------------------------------
# Stage 2: runtime
# ---------------------------------------------------------------------------
FROM debian:trixie-slim AS runtime

LABEL org.opencontainers.image.title="lite-upf" \
      org.opencontainers.image.description="Lightweight 5G User Plane Function" \
      org.opencontainers.image.source="https://github.com/SigDrakon/lite-upf" \
      org.opencontainers.image.licenses="Apache-2.0"

ENV DEBIAN_FRONTEND=noninteractive

# iptables  -> the forwarding path installs NAT/FORWARD rules
# iproute2  -> ip/iptool for interface and route inspection
# ca-certs  -> TLS for PFCP over TLS and the webconsole backend
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        iptables \
        iproute2 \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /lite-upf
RUN mkdir -p config/ log/

COPY --from=builder /out/lite-upf ./lite-upf

# Default configuration is baked in; mount your own over /lite-upf/config to
# override it.
COPY configs/config.yaml ./config/config.yaml

# PFCP / N4 (control plane) and GTP-U / N3 (access plane)
EXPOSE 8805/udp
EXPOSE 2152/udp

# The forwarding path needs NET_ADMIN to program routes and iptables rules,
# and NET_RAW for raw netlink sockets.
# Run with:  --cap-add=NET_ADMIN --cap-add=NET_RAW
VOLUME ["/lite-upf/config", "/lite-upf/log"]

ENTRYPOINT ["/lite-upf/lite-upf"]
CMD ["-config", "/lite-upf/config/config.yaml"]
