# Verdandi

Verdandi is a pure Go local multi-agent orchestration runtime with a real MCP stdio server.

## Quick Start

```bash
go install ./cmd/verdandi
go install ./cmd/verdandi-mcp
verdandi "계산기 프로그램 기획 구현 테스트 문서화"
```

Verdandi treats a plain sentence as the default command. The request is analyzed,
the required agents are selected, and the workflow runs automatically.

Analyze without running the workflow:

```bash
verdandi --analyze "작업을 분석하고 필요한 에이전트를 동적으로 생성해서 연계 실행"
```

Return machine-readable JSON:

```bash
verdandi --json "계산기 프로그램 기획 구현 테스트 문서화"
```

## MCP Server

Build the Go MCP server:

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

Available tools:

- `run` - default natural-language execution tool for LLMs
- `analyze`
- `orchestrate`
- `get_status`
- `open_output`

For normal use, an LLM only needs to call `run` with:

```json
{
  "request": "계산기 앱을 기획하고 구현하고 테스트해줘"
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
