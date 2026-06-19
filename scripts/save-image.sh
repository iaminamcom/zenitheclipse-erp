#!/usr/bin/env sh
set -eu
IMAGE_NAME="${IMAGE_NAME:-zenith-eclipse-erp:3.3.0-dokploy}"
OUT="${OUT:-zenith-eclipse-erp-3.3.0-dokploy-image.tar}"
docker save "$IMAGE_NAME" -o "$OUT"
echo "Saved Docker image to $OUT"
