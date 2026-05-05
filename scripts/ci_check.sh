#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

GO_FILES="$(find . -name '*.go' -not -path './.git/*' -not -path './.verdandi/*')"
UNFORMATTED="$(gofmt -l $GO_FILES)"
if [[ -n "$UNFORMATTED" ]]; then
  echo "Go files need gofmt:"
  echo "$UNFORMATTED"
  exit 1
fi

go test ./...
go build ./cmd/verdandi ./cmd/verdandi-mcp ./cmd/verdandi-spinning-wheel
bash scripts/mcp_stdio_smoke.sh
bash scripts/mcp_http_smoke.sh
tmpdist="$(mktemp -d "${TMPDIR:-/tmp}/verdandi-ci-release.XXXXXX")"
trap 'rm -rf "$tmpdist"' EXIT
VERDANDI_VERSION=ci VERDANDI_RELEASE_TARGETS=current VERDANDI_DIST_DIR="$tmpdist" bash scripts/release_build.sh
VERDANDI_VERSION=ci VERDANDI_DIST_DIR="$tmpdist" bash scripts/release_notes.sh
