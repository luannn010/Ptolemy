#!/usr/bin/env bash
# Bring up the Ptolemy Postgres container (ParadeDB = Postgres + pgvector +
# ParadeDB extensions). Run this on the host that owns /srv/ptolemy/pgdata
# (currently debian-sv1). The dev workstation reaches it via DATABASE_URL in
# .env at host port 1091.
#
# WARNING: the "start clean" block below is destructive — it wipes
# /srv/ptolemy/pgdata. Skip it when you want to preserve data; run it only
# when you need to escape a crash loop or reinitialise from scratch.
#
# Usage:
#   bash scripts/postgres-up.sh           # bring up, reusing existing pgdata
#   WIPE=1 bash scripts/postgres-up.sh    # nuke pgdata first, then bring up

set -euo pipefail

CONTAINER=ptolemy-postgres
DATA_DIR=/srv/ptolemy/pgdata
HOST_PORT=1091
IMAGE=paradedb/paradedb:latest

# Remove any prior container (stops a restart loop if one is running).
docker rm -f "$CONTAINER" 2>/dev/null || true

if [[ "${WIPE:-0}" == "1" ]]; then
  sudo rm -rf "$DATA_DIR"
  sudo mkdir -p "$DATA_DIR"
  sudo chown -R 999:999 "$DATA_DIR"
fi

docker run -d \
  --name "$CONTAINER" \
  --restart unless-stopped \
  -e POSTGRES_USER=ptolemy \
  -e POSTGRES_PASSWORD=ptolemy \
  -e POSTGRES_DB=ptolemy \
  -v "$DATA_DIR":/var/lib/postgresql \
  -p "$HOST_PORT":5432 \
  "$IMAGE"
