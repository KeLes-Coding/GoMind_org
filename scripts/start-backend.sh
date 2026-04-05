#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"
APP_BIN="$BIN_DIR/gomind"

ROLE="${ROLE:-all}"
GO_CMD="${GO_CMD:-go}"
GO_CACHE_DIR="${GOCACHE:-/tmp/go-build}"
DEFAULT_PROXY="${DEFAULT_PROXY:-http://127.0.0.1:7890}"
ENABLE_PROXY_AUTOFILL="${ENABLE_PROXY_AUTOFILL:-1}"
CHECK_DEEPSEEK_CONNECTIVITY="${CHECK_DEEPSEEK_CONNECTIVITY:-1}"

if [[ ! "$ROLE" =~ ^(server|worker|all)$ ]]; then
  echo "Invalid ROLE: $ROLE"
  echo "Allowed values: server, worker, all"
  exit 1
fi

if ! command -v "$GO_CMD" >/dev/null 2>&1; then
  echo "Go is not installed or not in PATH: $GO_CMD"
  exit 1
fi

if [[ "$ENABLE_PROXY_AUTOFILL" == "1" ]]; then
  if [[ -z "${HTTP_PROXY:-}" && -z "${http_proxy:-}" ]]; then
    export HTTP_PROXY="$DEFAULT_PROXY"
    export http_proxy="$DEFAULT_PROXY"
  fi
  if [[ -z "${HTTPS_PROXY:-}" && -z "${https_proxy:-}" ]]; then
    export HTTPS_PROXY="$DEFAULT_PROXY"
    export https_proxy="$DEFAULT_PROXY"
  fi
fi

mkdir -p "$BIN_DIR"
mkdir -p "$GO_CACHE_DIR"

echo "Proxy configuration:"
echo "  HTTP_PROXY=${HTTP_PROXY:-${http_proxy:-<empty>}}"
echo "  HTTPS_PROXY=${HTTPS_PROXY:-${https_proxy:-<empty>}}"

if [[ "$CHECK_DEEPSEEK_CONNECTIVITY" == "1" && "$ROLE" =~ ^(server|all)$ ]]; then
  if command -v curl >/dev/null 2>&1; then
    echo "Checking DeepSeek API connectivity..."
    if curl -I --silent --show-error --fail --max-time 8 https://api.deepseek.com >/dev/null; then
      echo "DeepSeek API connectivity: OK"
    else
      echo "DeepSeek API connectivity: FAILED"
      echo "Tip: verify proxy availability or export a reachable HTTP(S)_PROXY before starting the backend."
    fi
  else
    echo "curl not found, skipping DeepSeek API connectivity check"
  fi
fi

echo "Building backend binary..."
GOCACHE="$GO_CACHE_DIR" "$GO_CMD" build -o "$APP_BIN" .

echo "Starting backend with role=$ROLE"
exec "$APP_BIN" -role="$ROLE"
