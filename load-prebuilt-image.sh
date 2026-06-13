#!/usr/bin/env bash
set -euo pipefail
ARCH="${1:-amd64}"
TAR="ZenithEclipseERP_Docker_Image_${ARCH}.tar"
if [ ! -f "$TAR" ]; then
  echo "File not found: $TAR"
  echo "Put this script beside the image tar, or pass amd64/arm64 correctly."
  exit 1
fi
docker load -i "$TAR"
echo "Loaded zenitheclipse/erp-ultimate:1.0.0-${ARCH}"
