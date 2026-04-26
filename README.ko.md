# Verdandi

Verdandi는 자연어 요청을 로컬 실행 워크플로우로 바꾸는 순수 Go 기반
오케스트레이션 런타임입니다. CLI와 MCP stdio 서버를 함께 제공하므로 터미널과
MCP 지원 LLM 클라이언트에서 같은 실행 엔진을 사용할 수 있습니다.

## 무엇을 하나요?

- 자연어 요청을 분석해 실행 단계를 구성합니다.
- `planner`, `code-writer`, `tester`, `documenter`, `deployer` 단계로 작업을 나눕니다.
- 생성 결과와 실행 기록을 `.verdandi/` 아래에 저장합니다.
- MCP 도구 `run`, `analyze`, `orchestrate`, `get_status`, `open_output`을 제공합니다.
- 생성된 Go 프로젝트를 `go test ./...`로 검증합니다.
- 자연어 분석 backend를 `keyword`, `llm`, `auto` 중에서 선택할 수 있습니다.

## 빠른 시작

```bash
go install ./cmd/verdandi
go install ./cmd/verdandi-mcp
verdandi "계산기 앱을 기획하고 구현하고 테스트하고 문서화해줘"
```

설치 없이 실행하려면:

```bash
go run ./cmd/verdandi --json "기획 구현 테스트 문서화"
```

분석만 보고 싶다면:

```bash
go run ./cmd/verdandi --analyze "기획 구현 테스트 문서화"
```

## MCP 서버

```bash
go build -o bin/verdandi-mcp ./cmd/verdandi-mcp
```

MCP 클라이언트 설정 예시:

```json
{
  "mcpServers": {
    "verdandi": {
      "command": "verdandi-mcp"
    }
  }
}
```

일반적인 LLM 사용에서는 `run` 도구에 요청만 넘기면 됩니다.

```json
{
  "request": "계산기 앱을 기획하고 구현하고 테스트해줘"
}
```

## LLM 분석기

기본값은 로컬 `keyword` 분석기입니다. LLM에 자연어 해석을 맡기려면
OpenAI 호환 chat-completions 엔드포인트와 API 키를 설정하고 `llm` 또는 `auto`
분석기를 선택합니다.

```bash
export VERDANDI_ANALYZER=llm
export VERDANDI_LLM_ENDPOINT=https://example.com/v1/chat/completions
export VERDANDI_LLM_API_KEY=...
verdandi --analyze "계산기 앱을 만들고 품질 검증까지 해줘"
```

LLM이 반환한 단계는 Verdandi가 허용 목록으로 검증하며, 실패하면 keyword 분석기로
되돌아갑니다.

## 현재 범위

Verdandi는 현재 로컬 MVP 런타임입니다. 외부 에이전트 프로세스 실행은 아직
포함하지 않습니다. 대신 요청 분석, 실행 계획 preview, 로컬 파일 생성,
`go test ./...` 검증, 실행 기록 조회에 집중합니다.

## 개발 검증

```bash
go test ./...
go build ./cmd/verdandi
go build ./cmd/verdandi-mcp
```
