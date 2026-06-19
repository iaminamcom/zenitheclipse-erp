# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.23-bookworm AS build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/zenith-erp ./main.go

FROM debian:bookworm-slim AS runtime
LABEL org.opencontainers.image.title="Zenith Eclipse ERP"
LABEL org.opencontainers.image.description="Zenith Eclipse ERP A-to-Z document and operations system"
LABEL org.opencontainers.image.version="3.3.0-docker-dokploy"

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        tzdata \
        chromium \
        fonts-dejavu \
        fonts-liberation \
        fonts-noto-core \
        fonts-noto-cjk \
        fonts-noto-color-emoji \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/zenith-erp /app/zenith-erp

RUN useradd --system --uid 10001 --gid 0 --home-dir /data zenith \
    && mkdir -p /data/uploads /data/backups \
    && chown -R 10001:0 /data /app \
    && chmod -R g=u /data /app

USER 10001
ENV ZENITH_ERP_DOCKER=1 \
    ZENITH_ERP_HEADLESS=1 \
    ZENITH_ERP_HOME=/data \
    ZENITH_ERP_BIND=0.0.0.0 \
    ZENITH_ERP_PORT=8080 \
    ZENITH_ERP_URL_HOST=localhost \
    TZ=Asia/Dubai

EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD curl -fsS http://127.0.0.1:${ZENITH_ERP_PORT:-8080}/health | grep -q ok || exit 1

CMD ["/app/zenith-erp"]
