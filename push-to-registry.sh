#!/usr/bin/env bash
set -euo pipefail
# Example:
# REGISTRY_IMAGE=ghcr.io/YOUR-GITHUB-USER/zenith-erp IMAGE_TAG=1.0.0 ./push-to-registry.sh
REGISTRY_IMAGE="${REGISTRY_IMAGE:?Set REGISTRY_IMAGE, example ghcr.io/YOUR-GITHUB-USER/zenith-erp}"
IMAGE_TAG="${IMAGE_TAG:-1.0.0}"
docker buildx build --platform linux/amd64,linux/arm64 -t "$REGISTRY_IMAGE:$IMAGE_TAG" -t "$REGISTRY_IMAGE:latest" --push .
echo "Pushed $REGISTRY_IMAGE:$IMAGE_TAG and $REGISTRY_IMAGE:latest"
