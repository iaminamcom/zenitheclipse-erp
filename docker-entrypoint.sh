#!/bin/sh
set -eu

# Runtime volumes mask the ownership prepared in the image. Ensure a fresh
# Docker/Dokploy volume is writable by the unprivileged application user.
mkdir -p /data/uploads /data/backups
chown -R 10001:0 /data
chmod -R g=u /data

exec gosu zenith "$@"
