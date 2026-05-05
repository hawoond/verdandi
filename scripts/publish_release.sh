#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERDANDI_VERSION:-${GITHUB_REF_NAME:-dev}}"
VERSION="${VERSION#v}"
TAG="${VERDANDI_RELEASE_TAG:-v$VERSION}"
DIST_DIR="${VERDANDI_DIST_DIR:-$ROOT/dist}"
NOTES="$DIST_DIR/release-notes.md"

if [[ ! -f "$NOTES" ]]; then
  echo "missing release notes: $NOTES" >&2
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "gh CLI is required to publish GitHub releases" >&2
  exit 1
fi

assets=("$DIST_DIR"/*.tar.gz "$DIST_DIR"/*.zip "$DIST_DIR"/checksums.txt "$DIST_DIR"/manifest.json "$DIST_DIR"/sbom.spdx.json)

if gh release view "$TAG" >/dev/null 2>&1; then
  gh release upload "$TAG" "${assets[@]}" --clobber
  gh release edit "$TAG" --title "Verdandi $TAG" --notes-file "$NOTES"
else
  target_args=()
  if [[ -n "${GITHUB_SHA:-}" ]]; then
    target_args=(--target "$GITHUB_SHA")
  fi
  gh release create "$TAG" "${assets[@]}" "${target_args[@]}" --title "Verdandi $TAG" --notes-file "$NOTES"
fi
