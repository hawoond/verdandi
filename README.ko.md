# Verdandi

Verdandi는 자연어 요청을 로컬 실행 워크플로우로 바꾸는 순수 Go 기반
오케스트레이션 런타임입니다. CLI와 MCP stdio 서버를 함께 제공하므로 터미널과
MCP 지원 LLM 클라이언트에서 같은 실행 엔진을 사용할 수 있습니다.

## 무엇을 하나요?

- 자연어 요청을 분석해 실행 단계를 구성합니다.
- `planner`, `code-writer`, `tester`, `documenter`, `deployer` 단계로 작업을 나눕니다.
- `planner` 단계에서 요구사항, 수용 기준, 작업 분해, 리스크 문서를 생성합니다.
- 생성 결과와 실행 기록을 `.verdandi/` 아래에 저장합니다.
- MCP 도구 `run`, `run_plan`, `validate_plan`, `analyze`, `orchestrate`, `get_status`, `open_output`, `list_agents`를 제공합니다.
- MCP 리소스, 리소스 템플릿, 프롬프트를 제공해 클라이언트 중립적인 상태 조회와 재사용 워크플로우를 지원합니다.
- 실행 기록을 바탕으로 동적 agent contract와 lifecycle 추천을 관리합니다.
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

Streamable HTTP로 실행하려면 같은 서버에 HTTP listener를 켭니다.

```bash
verdandi-mcp -http 127.0.0.1:8080
```

기본 MCP endpoint는 `http://127.0.0.1:8080/mcp`입니다. 다른 path를 쓰려면
`-mcp-path`를 지정하면 됩니다.

HTTP로 노출할 때는 browser origin을 제한하고 bearer token을 요구할 수 있습니다.

```bash
export VERDANDI_MCP_HTTP_BEARER_TOKEN=change-me
verdandi-mcp -http 127.0.0.1:8080 -http-session -http-allowed-origin https://client.example
```

클라이언트는 각 HTTP 요청에 `Authorization: Bearer change-me`를 보내면 됩니다.
`-http-session`을 켜면 initialize 응답에 `MCP-Session-Id`가 포함되고,
클라이언트는 이후 요청에 이 헤더를 보내며 `DELETE /mcp`로 세션을 종료할 수
있습니다.

일반적인 LLM 사용에서는 `run` 도구에 요청만 넘기면 됩니다.

```json
{
  "request": "계산기 앱을 기획하고 구현하고 테스트해줘"
}
```

MCP 클라이언트의 LLM이 직접 워크플로우 단계를 판단할 수 있다면 `run_plan`을
사용할 수 있습니다. Verdandi는 클라이언트가 고른 단계를 검증하고 정규화한 뒤
실행합니다.

```json
{
  "request": "계산기 앱을 만들고 테스트하고 문서화해줘",
  "stages": [
    { "stage": "code-writer", "keyword": "client-llm" },
    { "stage": "tester", "keyword": "client-llm" },
    { "stage": "documenter", "keyword": "client-llm" }
  ]
}
```

실행하지 않고 정규화된 계획만 확인하려면 같은 형식으로 `validate_plan`을
호출하면 됩니다.

### 표준 MCP 표면

Verdandi는 stdio 기반 MCP protocol version `2025-11-25`를 대상으로 합니다.
`tools`, `resources`, `prompts` capability를 광고하며, 아직 구현하지 않은
list change notification과 subscription 기능은 비활성 상태로 둡니다.

리소스:

- `verdandi://runs`
- `verdandi://agents`
- `verdandi://runs/{runId}`
- `verdandi://runs/{runId}/events`
- `verdandi://runs/{runId}/output`

프롬프트:

- `plan-and-run`
- `validate-plan`
- `inspect-run`
- `inspect-failed-run`
- `choose-agent-lifecycle`

전체 MCP 제품 표면은 [docs/mcp-standard-compatibility.md](docs/mcp-standard-compatibility.md)에
정리되어 있습니다. MCP Inspector 스타일 점검에 쓸 JSON-RPC fixture는
[docs/mcp-inspector-fixtures.jsonl](docs/mcp-inspector-fixtures.jsonl)에 있습니다.
테스트가 고정하는 안정 계약 snapshot은
[docs/mcp-contract-snapshot.json](docs/mcp-contract-snapshot.json)에 있습니다.
빌드된 stdio 서버를 fixture로 재생하는 스모크 테스트는 다음처럼 실행할 수 있습니다.

```bash
bash scripts/mcp_stdio_smoke.sh
bash scripts/mcp_http_smoke.sh
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

## 관측성

Verdandi는 실행 기록, 단계 결과, agent metric, visualization event를 `.verdandi/`
아래에 기록합니다. MCP 리소스는 optional Spinning Wheel 시각화 도구가 스트리밍하는
동일한 상태를 노출합니다. 설정과 실행 방법은 [docs/spinning-wheel.md](docs/spinning-wheel.md)를
참고하세요.

## 현재 범위

Verdandi는 현재 로컬 MVP 런타임입니다. 외부 에이전트 프로세스 실행은 아직
포함하지 않습니다. 대신 요청 분석, 실행 계획 preview, 로컬 파일 생성,
`go test ./...` 검증, 실행 기록 조회, MCP 표준 상태 조회, 로컬 실행 시각화에
집중합니다.

## 개발 검증

```bash
go test ./...
go build ./cmd/verdandi
go build ./cmd/verdandi-mcp
bash scripts/mcp_stdio_smoke.sh
bash scripts/mcp_http_smoke.sh
```

CI와 같은 로컬 gate를 한 번에 실행하려면:

```bash
bash scripts/ci_check.sh
```

## 릴리스 패키징

로컬 릴리스 archive, checksum, release manifest, SPDX SBOM은 다음처럼 만들 수
있습니다.

```bash
VERDANDI_VERSION=0.1.0 bash scripts/release_build.sh
```

결과물은 `dist/`에 생성됩니다. `v0.1.0` 같은 tag push도 GitHub Actions에서
release artifact workflow를 실행하고, 같은 GitHub Release를 생성 또는 갱신한 뒤
모든 archive와 `checksums.txt`, `manifest.json`을 첨부합니다.
릴리스에는 공급망 검토를 위해 Go module graph에서 생성한 `sbom.spdx.json`도
함께 포함됩니다.

checksum을 확인한 뒤 다운로드한 archive를 설치할 수 있습니다. archive 옆에
`manifest.json`이 있으면 installer가 manifest의 archive hash와 실제 파일을
한 번 더 대조한 뒤 바이너리를 복사합니다.

```bash
VERDANDI_INSTALL_DIR="$HOME/.local/bin" bash scripts/install_release.sh dist/verdandi_0.1.0_linux_amd64.tar.gz
```

릴리스 빌드는 세 바이너리에 version metadata를 주입합니다.

```bash
verdandi --version
verdandi-mcp --version
verdandi-spinning-wheel --version
```
