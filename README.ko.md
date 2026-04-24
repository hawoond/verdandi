# Verdandi

Verdandi는 순수 Go로 만든 로컬 멀티 에이전트 오케스트레이션 런타임이며,
실제 MCP stdio 서버를 함께 제공합니다.

## 빠른 시작

```bash
go install ./cmd/verdandi
go install ./cmd/verdandi-mcp
verdandi "계산기 앱을 기획하고 구현하고 테스트하고 문서화해줘"
```

Verdandi는 일반 문장을 기본 명령으로 처리합니다. 요청을 분석하고,
필요한 에이전트를 선택한 뒤 워크플로우를 자동으로 실행합니다.

실행하지 않고 분석만 하려면:

```bash
verdandi --analyze "작업을 분석하고 필요한 에이전트를 동적으로 생성해서 연계 실행"
```

기계가 읽기 쉬운 JSON으로 결과를 받으려면:

```bash
verdandi --json "계산기 앱을 기획하고 구현하고 테스트하고 문서화해줘"
```

## MCP 서버

MCP 서버 빌드:

```bash
go build -o bin/verdandi-mcp ./cmd/verdandi-mcp
```

MCP 클라이언트 설정:

```json
{
  "mcpServers": {
    "verdandi": {
      "command": "verdandi-mcp"
    }
  }
}
```

## MCP 도구

- `run` - LLM이 자연어 요청을 실행할 때 쓰는 기본 도구
- `analyze`
- `orchestrate`
- `get_status`
- `open_output`

일반적인 사용에서는 LLM이 `run`에 요청만 넘기면 됩니다:

```json
{
  "request": "계산기 앱을 기획하고 구현하고 테스트해줘"
}
```

## 데이터

런타임 데이터는 아래 디렉터리에 저장됩니다:

```text
.verdandi/
```

## 검증

```bash
go test ./...
go build ./cmd/verdandi
go build ./cmd/verdandi-mcp
```
