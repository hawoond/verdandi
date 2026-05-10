#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERDANDI_VERSION:-}"
if [[ -z "$VERSION" ]]; then
  if git describe --tags --always --dirty >/dev/null 2>&1; then
    VERSION="$(git describe --tags --always --dirty)"
  else
    VERSION="dev"
  fi
fi
VERSION="${VERSION#v}"

DIST_DIR="${VERDANDI_DIST_DIR:-$ROOT/dist}"
NOTES="$DIST_DIR/release-notes.md"

mkdir -p "$DIST_DIR"
if [[ ! -f "$DIST_DIR/checksums.txt" ]]; then
  echo "missing $DIST_DIR/checksums.txt" >&2
  exit 1
fi
if [[ ! -f "$DIST_DIR/manifest.json" ]]; then
  echo "missing $DIST_DIR/manifest.json" >&2
  exit 1
fi
if [[ ! -f "$DIST_DIR/sbom.spdx.json" ]]; then
  echo "missing $DIST_DIR/sbom.spdx.json" >&2
  exit 1
fi

{
  echo "# Verdandi $VERSION"
  echo
  echo "## Install"
  echo
  echo "Download the archive for your OS and architecture, then unpack it."
  echo
  echo '```bash'
  echo "sha256sum -c checksums.txt"
  echo "tar -xzf verdandi_${VERSION}_linux_amd64.tar.gz"
  echo "cd verdandi_${VERSION}_linux_amd64"
  echo "VERDANDI_INSTALL_DIR=\"\$HOME/.local/bin\" bash install_release.sh ../verdandi_${VERSION}_linux_amd64.tar.gz"
  echo "verdandi --version"
  echo "verdandi-mcp --version"
  echo "verdandi-spinning-wheel --version"
  echo '```'
  echo
  echo "For Windows, download the matching \`.zip\` archive and run the \`.exe\` binaries. Each archive also includes \`docs/INSTALL.md\`."
  echo
  echo "## Upgrade"
  echo
  echo '```bash'
  echo "verdandi upgrade"
  echo "verdandi upgrade --version $VERSION"
  echo '```'
  echo
  echo "## Assets"
  echo
  for artifact in "$DIST_DIR"/verdandi_"$VERSION"_*.tar.gz "$DIST_DIR"/verdandi_"$VERSION"_*.zip; do
    if [[ -f "$artifact" ]]; then
      echo "- $(basename "$artifact")"
    fi
  done
  echo "- checksums.txt"
  echo "- manifest.json"
  echo "- sbom.spdx.json"
  echo
  echo "## Verify Checksums"
  echo
  echo '```bash'
  echo "sha256sum -c checksums.txt"
  echo '```'
  echo
  echo "## Build Manifest"
  echo
  echo "\`manifest.json\` records the Verdandi version, source commit, build date, target platform, archive format, and SHA256 for each release archive."
  echo
  echo "## Software Bill of Materials"
  echo
  echo "\`sbom.spdx.json\` is an SPDX 2.3 JSON document generated from the Go module graph for the release build."
  echo
  echo "## checksums.txt"
  echo
  echo '```text'
  cat "$DIST_DIR/checksums.txt"
  echo '```'
} > "$NOTES"

echo "release notes written to $NOTES"
