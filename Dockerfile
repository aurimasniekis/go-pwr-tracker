# syntax=docker/dockerfile:1

# Multi-stage build: Node builds the UI, Go compiles a static binary with the
# UI embedded, and the runtime image contains only that binary.
# Builder stages run on the build platform and cross-compile, so multi-arch
# images do not need emulation.

FROM --platform=$BUILDPLATFORM node:24-alpine@sha256:44fce696f38ba8c987c4db89954051000bbd05ffd8d8659ba15ed4a8602e937f AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=
ARG BUILD_DATE=
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/*.go ./web/
COPY --from=web /src/web/dist ./web/dist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags embedui \
      -ldflags "-s -w \
        -X github.com/aurimasniekis/go-pwr-tracker/internal/version.Version=${VERSION} \
        -X github.com/aurimasniekis/go-pwr-tracker/internal/version.Commit=${COMMIT} \
        -X github.com/aurimasniekis/go-pwr-tracker/internal/version.Date=${BUILD_DATE}" \
      -o /out/pwr-tracker ./cmd/pwr-tracker \
 && mkdir -p /out/data

# distroless/static: CA certificates and tzdata included, no shell, runs as
# uid 65532. The binary also embeds Go's tzdata as a fallback.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
LABEL org.opencontainers.image.title="pwr-tracker" \
      org.opencontainers.image.description="Power Hit Radio airplay tracker" \
      org.opencontainers.image.source="https://github.com/aurimasniekis/go-pwr-tracker" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/pwr-tracker /usr/local/bin/pwr-tracker
# Pre-create /data owned by the runtime user so a fresh named volume is writable.
COPY --from=build --chown=65532:65532 /out/data /data
ENV DATABASE_PATH=/data/pwr-tracker.db \
    LISTEN_ADDR=:8080
VOLUME ["/data"]
EXPOSE 8080
USER 65532:65532
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD ["/usr/local/bin/pwr-tracker", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/pwr-tracker"]
CMD ["serve"]
