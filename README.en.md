# Verdandi

Verdandi is a pure Go local orchestration runtime that turns natural-language
requests into small, inspectable workflows. It ships with both a CLI and a real
MCP stdio server, so the same runtime can be used from a terminal or an
MCP-capable LLM client.

## What It Does

- Analyzes natural-language requests and builds an execution plan.
- Splits work into `planner`, `code-writer`, `tester`, `documenter`, and `deployer` stages.
- Stores generated outputs and run history under `.verdandi/`.
- Exposes MCP tools: `run`, `analyze`, `orchestrate`, `get_status`, and `open_output`.
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

For normal LLM use, call the `run` tool with a request:

```json
{
  "request": "계산기 앱을 기획하고 구현하고 테스트해줘"
}
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

## Current Scope

Verdandi is currently a local MVP runtime. It does not spawn external agent
processes yet. Its focus is request analysis, execution plan previews, local
file generation, `go test ./...` validation, and run history lookup.

## Development Checks

```bash
go test ./...
go build ./cmd/verdandi
go build ./cmd/verdandi-mcp
```
