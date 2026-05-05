#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/verdandi-mcp-http-smoke.XXXXXX")"
trap 'if [[ -n "${SERVER_PID:-}" ]]; then kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; fi; rm -rf "$TMP_DIR"' EXIT

BIN="$TMP_DIR/verdandi-mcp"
DATA_DIR="$TMP_DIR/data"
SERVER_LOG="$TMP_DIR/server.log"
TOKEN="verdandi-smoke-token"

go build -o "$BIN" "$ROOT/cmd/verdandi-mcp"

PORT="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"
URL="http://127.0.0.1:${PORT}/mcp"

"$BIN" -data-dir "$DATA_DIR" -analyzer keyword -http "127.0.0.1:${PORT}" -http-session -http-bearer-token "$TOKEN" -http-allowed-origin "https://trusted.example" > "$TMP_DIR/stdout.log" 2> "$SERVER_LOG" &
SERVER_PID="$!"

python3 "$ROOT/scripts/validate_mcp_http_smoke.py" "$URL" "$TOKEN"
