#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/vue-frontend"

if ! command -v npm >/dev/null 2>&1; then
  echo "npm is not installed or not in PATH"
  exit 1
fi

cd "$FRONTEND_DIR"

if [[ ! -d node_modules ]]; then
  echo "Installing frontend dependencies..."
  npm ci
fi

echo "Starting frontend dev server..."
exec npm run serve
