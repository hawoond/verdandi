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
COMMIT="${VERDANDI_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_DATE="${VERDANDI_BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

DIST_DIR="${VERDANDI_DIST_DIR:-$ROOT/dist}"
TARGETS="${VERDANDI_RELEASE_TARGETS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64}"
if [[ "$TARGETS" == "current" ]]; then
  TARGETS="$(go env GOOS)/$(go env GOARCH)"
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/verdandi-release.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$DIST_DIR"
rm -f "$DIST_DIR"/verdandi_"$VERSION"_* "$DIST_DIR"/checksums.txt "$DIST_DIR"/manifest.json "$DIST_DIR"/sbom.spdx.json

commands=(verdandi verdandi-mcp verdandi-spinning-wheel)
LDFLAGS="-s -w -X github.com/genie-cvc/verdandi/internal/version.Version=$VERSION -X github.com/genie-cvc/verdandi/internal/version.Commit=$COMMIT -X github.com/genie-cvc/verdandi/internal/version.Date=$BUILD_DATE"

for target in $TARGETS; do
  GOOS_TARGET="${target%/*}"
  GOARCH_TARGET="${target#*/}"
  if [[ "$GOOS_TARGET" == "$GOARCH_TARGET" || -z "$GOOS_TARGET" || -z "$GOARCH_TARGET" ]]; then
    echo "invalid release target: $target" >&2
    exit 1
  fi

  package="verdandi_${VERSION}_${GOOS_TARGET}_${GOARCH_TARGET}"
  stage="$TMP_DIR/$package"
  mkdir -p "$stage/docs"

  ext=""
  if [[ "$GOOS_TARGET" == "windows" ]]; then
    ext=".exe"
  fi

  for command in "${commands[@]}"; do
    echo "building $command for $GOOS_TARGET/$GOARCH_TARGET"
    CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
      go build -trimpath -ldflags="$LDFLAGS" -o "$stage/$command$ext" "./cmd/$command"
  done

  cp README.md README.en.md README.ko.md "$stage/"
  cp scripts/install_release.sh "$stage/"
  cp docs/INSTALL.md docs/mcp-standard-compatibility.md docs/mcp-inspector-fixtures.jsonl docs/mcp-contract-snapshot.json "$stage/docs/"

  if [[ "$GOOS_TARGET" == "windows" ]]; then
    archive="$DIST_DIR/$package.zip"
    python3 - "$TMP_DIR" "$package" "$archive" <<'PY'
import os
import sys
import zipfile

root, package, archive = sys.argv[1:]
base = os.path.join(root, package)
with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as output:
    for current, _, files in os.walk(base):
        for name in files:
            path = os.path.join(current, name)
            output.write(path, os.path.relpath(path, root))
PY
  else
    archive="$DIST_DIR/$package.tar.gz"
    COPYFILE_DISABLE=1 tar -C "$TMP_DIR" -czf "$archive" "$package"
  fi
done

(
  cd "$DIST_DIR"
  for artifact in verdandi_"$VERSION"_*; do
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$artifact"
    else
      shasum -a 256 "$artifact"
    fi
  done
) > "$DIST_DIR/checksums.txt"

go list -m -json all > "$TMP_DIR/modules.json"
python3 - "$TMP_DIR/modules.json" "$DIST_DIR/sbom.spdx.json" "$VERSION" "$COMMIT" "$BUILD_DATE" <<'PY'
import json
import re
import sys
import urllib.parse

modules_path, sbom_path, version, commit, build_date = sys.argv[1:]

def read_modules(path):
    text = open(path, encoding="utf-8").read()
    decoder = json.JSONDecoder()
    modules = []
    offset = 0
    while offset < len(text):
        while offset < len(text) and text[offset].isspace():
            offset += 1
        if offset >= len(text):
            break
        module, offset = decoder.raw_decode(text, offset)
        modules.append(module)
    return modules

def spdx_id(value):
    clean = re.sub(r"[^A-Za-z0-9.-]+", "-", value).strip("-")
    return "SPDXRef-Package-" + (clean or "module")

def package_url(module_path, module_version):
    escaped_path = urllib.parse.quote(module_path, safe="/")
    if module_version and module_version != "UNKNOWN":
        return f"pkg:golang/{escaped_path}@{urllib.parse.quote(module_version, safe='')}"
    return f"pkg:golang/{escaped_path}"

packages = []
relationships = []

for module in read_modules(modules_path):
    module_path = module["Path"]
    module_version = version if module.get("Main") else module.get("Version", "UNKNOWN")
    package_id = spdx_id(f"{module_path}-{module_version}")
    packages.append({
        "SPDXID": package_id,
        "downloadLocation": "NOASSERTION",
        "externalRefs": [{
            "referenceCategory": "PACKAGE-MANAGER",
            "referenceLocator": package_url(module_path, module_version),
            "referenceType": "purl",
        }],
        "filesAnalyzed": False,
        "licenseConcluded": "NOASSERTION",
        "licenseDeclared": "NOASSERTION",
        "name": module_path,
        "versionInfo": module_version,
    })
    relationships.append({
        "relatedSpdxElement": package_id,
        "relationshipType": "DESCRIBES",
        "spdxElementId": "SPDXRef-DOCUMENT",
    })

sbom = {
    "SPDXID": "SPDXRef-DOCUMENT",
    "creationInfo": {
        "created": build_date,
        "creators": ["Tool: verdandi-release-build"],
    },
    "dataLicense": "CC0-1.0",
    "documentNamespace": f"https://github.com/genie-cvc/verdandi/spdx/verdandi-{version}-{commit}",
    "name": f"verdandi-{version}",
    "packages": packages,
    "relationships": relationships,
    "spdxVersion": "SPDX-2.3",
}

with open(sbom_path, "w", encoding="utf-8") as output:
    json.dump(sbom, output, indent=2, sort_keys=True)
    output.write("\n")
PY

python3 - "$DIST_DIR" "$VERSION" "$COMMIT" "$BUILD_DATE" <<'PY'
import json
import os
import sys

dist_dir, version, commit, build_date = sys.argv[1:]
prefix = f"verdandi_{version}_"
artifacts = []

with open(os.path.join(dist_dir, "checksums.txt"), encoding="utf-8") as checksums:
    for line in checksums:
        line = line.strip()
        if not line:
            continue
        sha256, name = line.split(None, 1)
        name = name.strip()
        if not name.startswith(prefix):
            continue
        if name.endswith(".tar.gz"):
            artifact_format = "tar.gz"
            target = name[:-7]
        elif name.endswith(".zip"):
            artifact_format = "zip"
            target = name[:-4]
        else:
            continue
        os_name, arch = target[len(prefix):].split("_", 1)
        artifacts.append({
            "name": name,
            "os": os_name,
            "arch": arch,
            "format": artifact_format,
            "sha256": sha256,
        })

manifest = {
    "product": "verdandi",
    "version": version,
    "commit": commit,
    "buildDate": build_date,
    "artifacts": artifacts,
}

with open(os.path.join(dist_dir, "manifest.json"), "w", encoding="utf-8") as output:
    json.dump(manifest, output, indent=2, sort_keys=True)
    output.write("\n")
PY

echo "release artifacts written to $DIST_DIR"
