# syntax=docker/dockerfile:1.7
# Zenith Eclipse ERP Ultimate - Docker image for Dokploy and normal Docker servers.

FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build
WORKDIR /src

# No external Go modules are required, but copy go.mod first for Docker layer caching.
COPY go.mod ./
COPY main.go ./
COPY web ./web

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/zenith-erp .

FROM alpine:3.20
RUN addgroup -S zenith && \
    adduser -S -G zenith zenith && \
    mkdir -p /app /data && \
    chown -R zenith:zenith /app /data

WORKDIR /app
COPY --from=build /out/zenith-erp /app/zenith-erp
RUN chmod +x /app/zenith-erp

ENV ZENITH_ERP_ADDR=0.0.0.0:8080 \
    ZENITH_ERP_BROWSER=0 \
    ZENITH_ERP_DATA=/data \
    ZENITH_ERP_COOKIE_SECURE=1

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

USER zenith
CMD ["/app/zenith-erp"]
