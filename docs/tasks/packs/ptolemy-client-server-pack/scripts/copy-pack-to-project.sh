#!/usr/bin/env bash
set -euo pipefail
DEST="${1:-.ptolemy/tasks/packs/client-server}"
mkdir -p "$DEST"
cp -R ./* "$DEST"/
echo "Copied pack to $DEST"
