# Spinning Wheel

Spinning Wheel is Verdandi's optional visualization plugin. It shows runs as a shared workspace where generated agents appear, move between stage zones, speak through status bubbles, and stream live execution events.

## Run Locally

```bash
go run ./cmd/verdandi-spinning-wheel --data-dir .verdandi --addr 127.0.0.1:8787
```

Open `http://127.0.0.1:8787/`.

## Generate Events

In another terminal:

```bash
go run ./cmd/verdandi --data-dir .verdandi --run "기획 구현 테스트 문서화"
```

Refresh Spinning Wheel or select the new run from the run dropdown.

## Live Mode

The browser connects to:

```text
/api/runs/{runId}/events/stream?follow=1
```

The server streams existing events first and then follows appended JSONL events. Heartbeats keep the connection alive when no new events are available.

## Troubleshooting

- If the page is stale, hard refresh the browser.
- If port `8787` is occupied, pass another `--addr`.
- If no runs appear, verify that both Verdandi and Spinning Wheel use the same `--data-dir`.
- If live status keeps reconnecting, check `/api/health` and the server terminal output.
