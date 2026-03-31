#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"
APP_BIN="$BIN_DIR/gomind"

ROLE="${ROLE:-server}"
GO_CMD="${GO_CMD:-go}"

if [[ ! "$ROLE" =~ ^(server|worker|all)$ ]]; then
  echo "Invalid ROLE: $ROLE"
  echo "Allowed values: server, worker, all"
  exit 1
fi

if ! command -v "$GO_CMD" >/dev/null 2>&1; then
  echo "Go is not installed or not in PATH: $GO_CMD"
  exit 1
fi

mkdir -p "$BIN_DIR"

echo "Building backend binary..."
"$GO_CMD" build -o "$APP_BIN" .

echo "Starting backend with role=$ROLE"
exec "$APP_BIN" -role="$ROLE"
