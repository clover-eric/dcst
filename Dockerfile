# syntax=docker/dockerfile:1.7

ARG GO_IMAGE=golang:1.27-alpine
ARG ALPINE_IMAGE=alpine:3.22

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT=

ENV CGO_ENABLED=0
WORKDIR /src

COPY engine/go.mod engine/go.sum ./engine/
COPY go.mod go.sum ./
RUN cd /src/engine && go mod download && \
    cd /src && go mod download

COPY . .

RUN cd /src/engine && \
    GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath \
      -ldflags="-s -w -X main.version=v2.3.5-ultra.1" \
      -o /out/cfst .

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath \
      -ldflags="-s -w" \
      -o /out/dcst ./cmd/dcst

FROM ${ALPINE_IMAGE}

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S -g 1000 dcst && \
    adduser -S -D -H -u 1000 -G dcst dcst && \
    mkdir -p /app /data && \
    chown -R dcst:dcst /data

WORKDIR /app
COPY --from=build /out/dcst /app/dcst
COPY --from=build /out/cfst /app/cfst
COPY engine/ip.txt engine/ipv6.txt /app/

ENV DATA_DIR=/data \
    LISTEN_ADDR=:10123 \
    CFST_PATH=/app/cfst \
    BUNDLED_IP_DIR=/app \
    INTERVAL_MINUTES=60 \
    CANDIDATES=40 \
    STREAMS=16 \
    TEST_DURATION_SECONDS=5 \
    WARMUP_SECONDS=1 \
    TEST_PORT=443 \
    TEST_URL=https://cf.xiu2.xyz/url \
    MIN_SPEED_MB_S=0 \
    MAX_LATENCY_MS=9999 \
    MAX_LOSS_RATE=1 \
    USE_IPV6=true \
    RUN_ON_START=true

VOLUME ["/data"]
EXPOSE 10123
USER 1000:1000

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -q -O - http://127.0.0.1:10123/health >/dev/null || exit 1

ENTRYPOINT ["/app/dcst"]
