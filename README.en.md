# Verdandi

Verdandi is a pure Go local orchestration runtime that turns natural-language
requests into small, inspectable workflows. It ships with both a CLI and a real
MCP stdio server, so the same runtime can be used from a terminal or an
MCP-capable LLM client.

## What It Does

- Analyzes natural-language requests and builds an execution plan.
- Splits work into `planner`, `code-writer`, `tester`, `documenter`, and `deployer` stages.
- Produces structured planner artifacts: requirements, acceptance criteria, task breakdown, and risks.
- Stores generated outputs and run history under `.verdandi/`.
- Exposes MCP tools: `run`, `run_plan`, `validate_plan`, `analyze`, `orchestrate`, `get_status`, `open_output`, and `list_agents`.
- Exposes MCP resources, resource templates, and prompts for client-neutral state discovery and reusable workflows.
- Tracks dynamic agent contracts and lifecycle recommendations across runs.
- Validates generated Go projects with `go test ./...`.
- Selects the request analyzer backend from `keyword`, `llm`, or `auto`.

## Quick Start

```bash
go install ./cmd/verdandi
go install ./cmd/verdandi-mcp
verdandi "계산기 앱을 기획하고 구현하고 테스트하고 문서화해줘"
```

Run without installing:

```bash
go run ./cmd/verdandi --json "기획 구현 테스트 문서화"
```

Preview the plan without executing:

```bash
go run ./cmd/verdandi --analyze "기획 구현 테스트 문서화"
```

## MCP Server

```bash
go build -o bin/verdandi-mcp ./cmd/verdandi-mcp
```

Example MCP client config:

```json
{
  "mcpServers": {
    "verdandi": {
      "command": "verdandi-mcp"
    }
  }
}
```

For Streamable HTTP, run the same server with an HTTP listener:

```bash
verdandi-mcp -http 127.0.0.1:8080
```

The MCP endpoint is `http://127.0.0.1:8080/mcp` by default. Use `-mcp-path` to
choose a different endpoint path.

For exposed HTTP deployments, restrict browser origins and require a bearer
token:

```bash
export VERDANDI_MCP_HTTP_BEARER_TOKEN=change-me
verdandi-mcp -http 127.0.0.1:8080 -http-session -http-allowed-origin https://client.example
```

Clients then send `Authorization: Bearer change-me` on each HTTP request. With
`-http-session`, the initialize response includes `MCP-Session-Id`; clients send
that header on later requests and can terminate the session with `DELETE /mcp`.

For normal LLM use, call the `run` tool with a request:

```json
{
  "request": "계산기 앱을 기획하고 구현하고 테스트해줘"
}
```

If your MCP client LLM can choose the workflow stages itself, call `run_plan`
instead. Verdandi will validate and normalize the client-selected plan before
executing it:

```json
{
  "request": "build a calculator app, test it, and write documentation",
  "stages": [
    { "stage": "code-writer", "keyword": "client-llm" },
    { "stage": "tester", "keyword": "client-llm" },
    { "stage": "documenter", "keyword": "client-llm" }
  ]
}
```

Use `validate_plan` with the same shape when you want to check the normalized
plan without executing it.

### Standard MCP Surface

Verdandi targets MCP protocol version `2025-11-25` over stdio. It advertises
`tools`, `resources`, and `prompts` capabilities, while keeping optional list
change and subscription features disabled until they are implemented.

Resources:

- `verdandi://runs`
- `verdandi://agents`
- `verdandi://runs/{runId}`
- `verdandi://runs/{runId}/events`
- `verdandi://runs/{runId}/output`

Prompts:

- `plan-and-run`
- `validate-plan`
- `inspect-run`
- `inspect-failed-run`
- `choose-agent-lifecycle`

See [docs/mcp-standard-compatibility.md](docs/mcp-standard-compatibility.md) for
the full MCP product surface. JSON-RPC fixture requests for MCP Inspector-style
checks are in [docs/mcp-inspector-fixtures.jsonl](docs/mcp-inspector-fixtures.jsonl).
The stable contract snapshot used by tests is
[docs/mcp-contract-snapshot.json](docs/mcp-contract-snapshot.json).
You can replay those fixtures against the built stdio server with:

```bash
bash scripts/mcp_stdio_smoke.sh
bash scripts/mcp_http_smoke.sh
```

## LLM Analyzer

The default backend is the local `keyword` analyzer. To delegate natural-language
interpretation to an LLM, configure an OpenAI-compatible chat-completions
endpoint and API key, then select `llm` or `auto`.

```bash
export VERDANDI_ANALYZER=llm
export VERDANDI_LLM_ENDPOINT=https://example.com/v1/chat/completions
export VERDANDI_LLM_API_KEY=...
verdandi --analyze "build a calculator app and validate quality"
```

Verdandi validates LLM-proposed stages against its allowlist and falls back to
the keyword analyzer if the LLM response is unavailable or invalid.

## Observability

Verdandi records run history, stage results, agent metrics, and visualization
events under `.verdandi/`. MCP resources expose the same state that the optional
Spinning Wheel visualizer streams from [docs/spinning-wheel.md](docs/spinning-wheel.md).

## Current Scope

Verdandi is currently a local MVP runtime. It does not spawn external agent
processes yet. Its focus is request analysis, execution plan previews, local
file generation, `go test ./...` validation, run history lookup, MCP-standard
state discovery, and local run visualization.

## Development Checks

```bash
go test ./...
go build ./cmd/verdandi
go build ./cmd/verdandi-mcp
bash scripts/mcp_stdio_smoke.sh
bash scripts/mcp_http_smoke.sh
```

To run the same local gate as CI:

```bash
bash scripts/ci_check.sh
```

## Release Packaging

Build local release archives, checksums, a release manifest, and an SPDX SBOM
with:

```bash
VERDANDI_VERSION=0.1.0 bash scripts/release_build.sh
```

Artifacts are written to `dist/`. Tag pushes like `v0.1.0` also run the release
artifact workflow in GitHub Actions, create or update the matching GitHub
Release, and attach all archives plus `checksums.txt` and `manifest.json`.
The release also includes `sbom.spdx.json`, generated from the Go module graph
for supply-chain review.

Install a downloaded archive after verifying checksums. When `manifest.json` is
next to the archive, the installer cross-checks the archive hash against the
manifest before copying binaries:

```bash
VERDANDI_INSTALL_DIR="$HOME/.local/bin" bash scripts/install_release.sh dist/verdandi_0.1.0_linux_amd64.tar.gz
```

Release builds inject version metadata into all three binaries:

```bash
verdandi --version
verdandi-mcp --version
verdandi-spinning-wheel --version
```
