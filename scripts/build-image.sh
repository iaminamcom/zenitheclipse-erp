#!/usr/bin/env sh
set -eu
IMAGE_NAME="${IMAGE_NAME:-zenith-eclipse-erp:3.3.0-dokploy}"
docker build -t "$IMAGE_NAME" .
echo "Built $IMAGE_NAME"
