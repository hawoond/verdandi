#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/verdandi-mcp-smoke.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT

BIN="$TMP_DIR/verdandi-mcp"
DATA_DIR="$TMP_DIR/data"
OUT="$TMP_DIR/stdout.jsonl"
ERR="$TMP_DIR/stderr.log"
FIXTURE="$ROOT/docs/mcp-inspector-fixtures.jsonl"

go build -o "$BIN" "$ROOT/cmd/verdandi-mcp"

"$BIN" -data-dir "$DATA_DIR" -analyzer keyword < "$FIXTURE" > "$OUT" 2> "$ERR"

python3 "$ROOT/scripts/validate_mcp_stdio_smoke.py" "$FIXTURE" "$OUT" "$ERR"
