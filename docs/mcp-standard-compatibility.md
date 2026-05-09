# MCP Standard Compatibility

Verdandi's MCP server targets the Model Context Protocol `2025-11-25` protocol version over stdio and Streamable HTTP. The server is designed around standard MCP surfaces instead of client-specific behavior, so clients can discover tools, resources, resource templates, and prompts through normal JSON-RPC requests.

## Capabilities

`initialize` advertises:

- `tools` with `listChanged: false`
- `resources` with `listChanged: false`
- `prompts` with `listChanged: false`
- `completions`

Verdandi does not currently advertise resource subscriptions or prompt list change notifications.

## Tools

Verdandi exposes these MCP tools:

- `run`: let Verdandi analyze a natural-language request and prepare a workflow package for an external LLM coding agent.
- `prepare_workflow`: prepare a reusable agent/skill workflow package without using the compatibility `run` action name.
- `recommend_assets`: list active reusable agent and skill assets for a request.
- `record_outcome`: record success, failure, error text, and lessons for a persisted asset.
- `run_plan`: run a client-selected ordered stage plan after Verdandi validates and normalizes it.
- `validate_plan`: validate and normalize a client-selected plan without executing it.
- `analyze`: inspect Verdandi's own analyzer output without execution.
- `orchestrate`: compatibility alias for `run`.
- `get_status`: read a previous run record by `runId`.
- `open_output`: list generated output files for a previous run.
- `list_agents`: list persisted agent contracts and lifecycle recommendations.
- `list_skills`: list persisted skill assets.

Tool execution failures are returned as tool results with `isError: true` so clients can retry with corrected arguments. Protocol-level problems such as unknown tools still return JSON-RPC errors.

## Resources

Static resources:

- `verdandi://runs`: persisted run records.
- `verdandi://agents`: persisted agent contracts and lifecycle recommendations.
- `verdandi://assets`: persisted agent and skill assets.
- `verdandi://skills`: persisted skill assets.

Resource templates:

- `verdandi://runs/{runId}`: one run record.
- `verdandi://runs/{runId}/events`: visualization events for a run.
- `verdandi://runs/{runId}/output`: output file metadata for a run.
- `verdandi://workflows/{runId}`: one prepared workflow package.
- `verdandi://workflows/{runId}/handoff`: Markdown handoff prompt for a prepared workflow.

Unknown resource URIs return JSON-RPC `-32002` with the requested URI in `error.data`.

`resources/list` returns the full resource list when no cursor is provided. When
`params.cursor` is present, Verdandi returns a page and includes `nextCursor`
when another page is available.

## Prompts

Verdandi exposes workflow prompts that help client LLMs use the tools and resources consistently:

- `plan-and-run`
- `validate-plan`
- `inspect-run`
- `inspect-failed-run`
- `choose-agent-lifecycle`

Prompts are user-controlled templates. They do not execute workflows by themselves.

`prompts/list` returns the full prompt list when no cursor is provided. When
`params.cursor` is present, Verdandi returns a page and includes `nextCursor`
when another page is available.

## Completions

Verdandi supports `completion/complete` for prompt arguments and resource
template arguments. Supported prompt completions:

- `request` for `plan-and-run`, `validate-plan`, and `choose-agent-lifecycle`
- `runId` for `inspect-run` and `inspect-failed-run`

Supported resource template completions:

- `runId` for `verdandi://runs/{runId}`
- `runId` for `verdandi://runs/{runId}/events`
- `runId` for `verdandi://runs/{runId}/output`
- `runId` for `verdandi://workflows/{runId}`
- `runId` for `verdandi://workflows/{runId}/handoff`

Suggestions are prefix-filtered, capped at 100 values, and sourced only from
local `.verdandi` run history. Workflow resource completions only suggest runs
that have prepared workflow package metadata. Completion values intentionally
avoid generated file paths, output contents, event payloads, and agent internals.

## Progress

When a `tools/call` request includes `_meta.progressToken`, Verdandi may emit
`notifications/progress` messages before the final tool response. Progress
notifications are currently emitted for `run`, `run_plan`, and `orchestrate`
workflow preparation or stage lifecycle updates. Each notification includes:

- `progressToken`: the original token supplied by the client
- `progress`: the number of completed stages
- `total`: the normalized stage count
- `message`: a stage lifecycle message such as `code-writer started`

Requests without `_meta.progressToken` preserve the previous quiet behavior and
return only the final JSON-RPC response.

## Cancellation

Verdandi accepts `notifications/cancelled` with `params.requestId` and optional
`params.reason`. Cancellation notifications are fire-and-forget and never
produce a JSON-RPC response.

For stdio `tools/call` requests that are still in flight, Verdandi records the
cancellation and suppresses the final response for that request. When the active
executor supports context-aware execution, Verdandi also propagates cancellation
into the local workflow context; tester validation uses `go test` through that
context. Unknown, already-completed, malformed, and late cancellation
notifications are ignored.

## Inspector Fixture

`docs/mcp-inspector-fixtures.jsonl` contains newline-delimited JSON-RPC messages covering initialization, tool discovery, resource discovery, cursor pagination, resource reads, prompt discovery, prompt rendering, completions, progress notifications, cancellation notifications, client-selected plan validation, and stdio JSON-RPC batch handling.

You can replay the file against `verdandi-mcp` with any stdio-capable MCP harness. The repository test suite also parses each fixture line and verifies that the MCP server handles it without JSON-RPC errors.

## Contract Snapshot

`docs/mcp-contract-snapshot.json` records the stable MCP product contract:
initialize capabilities, tool schemas, resources, resource templates, prompts,
supported completion references, and transport feature flags. The test suite
compares the live server output against this snapshot so accidental client-facing
changes fail fast. Intentional contract changes should update the snapshot in the
same patch as the implementation.

For a local process-level smoke test that builds `verdandi-mcp`, replays the
fixture over stdin/stdout, and validates responses, run:

```bash
bash scripts/mcp_stdio_smoke.sh
```

## Transport Notes

The stdio server accepts both single JSON-RPC requests and JSON-RPC batch arrays. Batch responses include only messages that require responses; notifications inside a batch are ignored as notifications.

The Streamable HTTP transport is available with:

```bash
verdandi-mcp -http 127.0.0.1:8080
```

The default MCP endpoint is `POST /mcp`. `POST` requests must include
`Accept: application/json, text/event-stream`. Requests that produce progress
notifications are returned as `text/event-stream`; quiet requests return
`application/json`. Notifications that do not require a JSON-RPC response return
HTTP `202 Accepted`.

Verdandi returns HTTP `405 Method Not Allowed` for standalone `GET /mcp` SSE
streams because it does not currently send unrelated server-initiated messages
outside a client request. It also validates `Origin` for browser-originated
requests and allows absent origins, `localhost`, and loopback IP origins by
default.

Additional trusted browser origins can be configured with
`-http-allowed-origin`, using a comma-separated list. HTTP requests can also be
protected with a shared bearer token:

```bash
export VERDANDI_MCP_HTTP_BEARER_TOKEN=change-me
verdandi-mcp -http 127.0.0.1:8080 -http-session -http-allowed-origin https://client.example
```

This is a local deployment guard, not a full OAuth authorization server. When
the token is configured, clients must send `Authorization: Bearer <token>` on
every HTTP request. Missing or invalid tokens receive HTTP `401 Unauthorized`
with a `WWW-Authenticate: Bearer` challenge.

When `-http-session` is enabled, Verdandi issues `MCP-Session-Id` on successful
HTTP initialize responses. Later HTTP requests must include that header. Missing
session IDs receive HTTP `400 Bad Request`; unknown or terminated session IDs
receive HTTP `404 Not Found`; `DELETE /mcp` with a valid session ID terminates
the session and returns HTTP `204 No Content`.

For a local process-level smoke test that builds `verdandi-mcp`, starts the HTTP
server, and validates JSON, SSE progress, notification, Accept, Origin, bearer
token, and session behavior, run:

```bash
bash scripts/mcp_http_smoke.sh
```

## Product Integration

The MCP server ties together the current Verdandi runtime features:

- `run` and `prepare_workflow` create workflow packages for external LLM coding agents.
- Workflow handoffs, selected assets, and task graphs are discoverable through workflow resources.
- Client-selected plans use `validate_plan` and `run_plan`.
- Agent lifecycle recommendations are exposed through `list_agents` and `verdandi://agents`.
- Persistent agent and skill assets are exposed through `recommend_assets`, `list_skills`, `verdandi://assets`, and `verdandi://skills`.
- Spinning Wheel consumes the same persisted runs and events that MCP resources expose.
