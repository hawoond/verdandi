# Verdandi

Verdandi is a pure Go local multi-agent orchestration runtime with a real MCP stdio server.

## Quick Start

```bash
go install ./cmd/verdandi
go install ./cmd/verdandi-mcp
verdandi "plan, implement, test, and document a calculator app"
```

Verdandi treats a plain sentence as the default command. The request is analyzed,
the required agents are selected, and the workflow runs automatically.

Analyze without running the workflow:

```bash
verdandi --analyze "analyze this task and dynamically coordinate the required agents"
```

Return machine-readable JSON:

```bash
verdandi --json "plan, implement, test, and document a calculator app"
```

## MCP Server

Build the MCP server:

```bash
go build -o bin/verdandi-mcp ./cmd/verdandi-mcp
```

MCP client config:

```json
{
  "mcpServers": {
    "verdandi": {
      "command": "verdandi-mcp"
    }
  }
}
```

## MCP Tools

- `run` - default natural-language execution tool for LLMs
- `analyze`
- `orchestrate`
- `get_status`
- `open_output`

For normal use, an LLM only needs to call `run` with:

```json
{
  "request": "plan, implement, test, and document a calculator app"
}
```

## Data

Runtime data is stored under:

```text
.verdandi/
```

## Verification

```bash
go test ./...
go build ./cmd/verdandi
go build ./cmd/verdandi-mcp
```
