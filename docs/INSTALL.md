# Install Verdandi

Download the archive for your OS and architecture from the GitHub Release, plus
`checksums.txt`, `manifest.json`, and `sbom.spdx.json`.

## Verify And Install

```bash
sha256sum -c checksums.txt
bash scripts/install_release.sh verdandi_VERSION_OS_ARCH.tar.gz
```

On macOS, `shasum -a 256 -c checksums.txt` is also supported by the install
script when `sha256sum` is unavailable. If `manifest.json` is next to the
archive, the install script also verifies that the archive hash matches the
release manifest entry before installing binaries.

`sbom.spdx.json` is an SPDX 2.3 software bill of materials generated from the Go
module graph for the release build. It is not required by the installer, but it
is published with each release for supply-chain review.

To install somewhere other than `/usr/local/bin`:

```bash
VERDANDI_INSTALL_DIR="$HOME/.local/bin" bash scripts/install_release.sh verdandi_VERSION_OS_ARCH.tar.gz
```

## Upgrade

After one release is installed, the `verdandi` CLI can fetch and install the
matching archive for your OS and architecture from GitHub Releases:

```bash
verdandi upgrade
```

Useful options:

```bash
verdandi upgrade --dry-run
verdandi upgrade --version 0.0.2
verdandi upgrade --install-dir "$HOME/.local/bin"
verdandi upgrade --force
```

The upgrade command downloads `checksums.txt` and `manifest.json`, verifies the
selected archive hash against both files, then installs `verdandi`,
`verdandi-mcp`, and `verdandi-spinning-wheel`.

## Check Installed Binaries

```bash
verdandi --version
verdandi-mcp --version
verdandi-spinning-wheel --version
```

## MCP Examples

stdio:

```json
{
  "mcpServers": {
    "verdandi": {
      "command": "verdandi-mcp"
    }
  }
}
```

Streamable HTTP:

```bash
export VERDANDI_MCP_HTTP_BEARER_TOKEN=change-me
verdandi-mcp -http 127.0.0.1:8080 -http-session -http-allowed-origin https://client.example
```
