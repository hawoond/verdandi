#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: install_release.sh ARCHIVE_PATH" >&2
  echo "Set VERDANDI_INSTALL_DIR to choose the install directory. Default: /usr/local/bin" >&2
}

if [[ "${1:-}" == "" || "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

ARCHIVE="$1"
INSTALL_DIR="${VERDANDI_INSTALL_DIR:-/usr/local/bin}"
ARCHIVE_DIR="$(cd "$(dirname "$ARCHIVE")" && pwd)"
ARCHIVE_NAME="$(basename "$ARCHIVE")"
CHECKSUMS="$ARCHIVE_DIR/checksums.txt"
MANIFEST="$ARCHIVE_DIR/manifest.json"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/verdandi-install.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT

if [[ ! -f "$ARCHIVE" ]]; then
  echo "archive not found: $ARCHIVE" >&2
  exit 1
fi
if [[ ! -f "$CHECKSUMS" ]]; then
  echo "checksums not found next to archive: $CHECKSUMS" >&2
  exit 1
fi

(
  cd "$ARCHIVE_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    grep "  $ARCHIVE_NAME$" checksums.txt | sha256sum -c -
  else
    grep "  $ARCHIVE_NAME$" checksums.txt | shasum -a 256 -c -
  fi
)

if [[ -f "$MANIFEST" ]]; then
  python3 - "$MANIFEST" "$ARCHIVE" "$ARCHIVE_NAME" <<'PY'
import hashlib
import json
import sys

manifest_path, archive_path, archive_name = sys.argv[1:]

with open(manifest_path, encoding="utf-8") as source:
    manifest = json.load(source)

expected = None
for artifact in manifest.get("artifacts", []):
    if artifact.get("name") == archive_name:
        expected = artifact.get("sha256")
        break

if not expected:
    sys.stderr.write(f"manifest missing artifact: {archive_name}\n")
    sys.exit(1)

digest = hashlib.sha256()
with open(archive_path, "rb") as source:
    for chunk in iter(lambda: source.read(1024 * 1024), b""):
        digest.update(chunk)

actual = digest.hexdigest()
if actual != expected:
    sys.stderr.write(f"manifest checksum mismatch for {archive_name}\n")
    sys.exit(1)
PY
fi

case "$ARCHIVE_NAME" in
  *.tar.gz)
    tar -C "$TMP_DIR" -xzf "$ARCHIVE"
    ;;
  *.zip)
    python3 - "$ARCHIVE" "$TMP_DIR" <<'PY'
import sys
import zipfile

archive, target = sys.argv[1:]
with zipfile.ZipFile(archive) as source:
    source.extractall(target)
PY
    ;;
  *)
    echo "unsupported archive type: $ARCHIVE_NAME" >&2
    exit 1
    ;;
esac

PACKAGE_DIR="$(find "$TMP_DIR" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
if [[ -z "$PACKAGE_DIR" ]]; then
  echo "archive did not contain a package directory" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
for binary in verdandi verdandi-mcp verdandi-spinning-wheel; do
  source="$PACKAGE_DIR/$binary"
  if [[ ! -f "$source" && -f "$source.exe" ]]; then
    source="$source.exe"
  fi
  if [[ ! -f "$source" ]]; then
    echo "missing binary in archive: $binary" >&2
    exit 1
  fi
  install -m 0755 "$source" "$INSTALL_DIR/$(basename "$source" .exe)"
done

echo "installed Verdandi binaries to $INSTALL_DIR"
