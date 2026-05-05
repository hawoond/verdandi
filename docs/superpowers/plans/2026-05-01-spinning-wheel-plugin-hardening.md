# Spinning Wheel Plugin Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn Spinning Wheel from a local visualizer command into an optional Verdandi visualization plugin with configurable startup, resilient live event streaming, richer agent activity presentation, and documented operating flows.

**Architecture:** Keep the current Go standard-library server and embedded web UI. Add a small plugin configuration layer around `internal/spinningwheel`, expose a plugin status API, make live streaming more robust with heartbeats and cursor support, then add UI layers for filtering, focus, and run health without changing Verdandi's core execution model.

**Tech Stack:** Go 1.22, Go standard library HTTP/SSE, embedded static assets, vanilla JavaScript canvas UI, existing Verdandi run/event store, table-driven Go tests with `httptest`.

---

## Execution Preconditions

- Start from `develop`.
- Create each implementation branch from `develop` with the `feature/` prefix.
- Merge completed work back into `develop`; do not merge into `main` until the final version is explicitly approved.
- Do not push unless the user asks.
- Keep `cmd/verdandi-spinning-wheel` runnable with:

```bash
go run ./cmd/verdandi-spinning-wheel --data-dir .verdandi --addr 127.0.0.1:8787
```

- After each task, run:

```bash
go test ./...
node --check internal/spinningwheel/web/app.js
git diff --check
```

## Current State

- `cmd/verdandi-spinning-wheel/main.go` starts the visualizer HTTP server.
- `internal/spinningwheel/server.go` serves static UI, run list JSON, run events JSON, and SSE live event streams.
- `internal/verdandi/events.go` writes visualization JSONL events and now supports append-style live events.
- `internal/spinningwheel/web/app.js` renders the map, agents, movement, speech bubbles, decision links, playback controls, roster, conversation log, and live stream connection.
- There is no explicit plugin manifest, plugin status endpoint, health endpoint, UI filter model, or operator guide yet.

## File Structure

- Create `internal/spinningwheel/config.go`
  - Own plugin configuration defaults and flag/env parsing helpers.
- Create `internal/spinningwheel/config_test.go`
  - Cover defaults and explicit configuration values.
- Modify `cmd/verdandi-spinning-wheel/main.go`
  - Use the new config object and print startup details.
- Modify `internal/spinningwheel/server.go`
  - Add `/api/plugin`, `/api/health`, and cursor-aware SSE behavior.
- Modify `internal/spinningwheel/server_test.go`
  - Cover plugin status, health, SSE heartbeats, and cursor filtering.
- Modify `internal/spinningwheel/web/index.html`
  - Add compact controls for live mode, stage filter, and focused agent.
- Modify `internal/spinningwheel/web/styles.css`
  - Add styles for status badges, filters, selected agents, and stream warnings.
- Modify `internal/spinningwheel/web/app.js`
  - Add plugin status loading, event filters, focus mode, cursor tracking, and live reconnect messaging.
- Create `docs/spinning-wheel.md`
  - Document how to run, test, and operate the plugin.
- Modify `README.md`
  - Keep README as an introduction page and link to `docs/spinning-wheel.md`.

## Task 1: Add Plugin Configuration Surface

**Files:**
- Create: `internal/spinningwheel/config.go`
- Create: `internal/spinningwheel/config_test.go`
- Modify: `cmd/verdandi-spinning-wheel/main.go`

- [ ] **Step 1: Write failing config tests**

Create `internal/spinningwheel/config_test.go`:

```go
package spinningwheel

import "testing"

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Name != "Spinning Wheel" {
		t.Fatalf("expected plugin name, got %#v", config)
	}
	if config.Enabled != true {
		t.Fatalf("expected plugin enabled by default")
	}
	if config.DataDir == "" {
		t.Fatalf("expected default data dir")
	}
	if config.Addr != "127.0.0.1:8787" {
		t.Fatalf("expected default addr, got %q", config.Addr)
	}
	if config.StreamPollInterval <= 0 {
		t.Fatalf("expected positive stream poll interval")
	}
}

func TestConfigWithOverrides(t *testing.T) {
	config := DefaultConfig().
		WithDataDir("/tmp/verdandi").
		WithAddr("127.0.0.1:9898").
		WithEnabled(false)

	if config.DataDir != "/tmp/verdandi" {
		t.Fatalf("expected override data dir, got %q", config.DataDir)
	}
	if config.Addr != "127.0.0.1:9898" {
		t.Fatalf("expected override addr, got %q", config.Addr)
	}
	if config.Enabled {
		t.Fatalf("expected disabled config")
	}
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
go test ./internal/spinningwheel -run 'TestDefaultConfig|TestConfigWithOverrides' -count=1
```

Expected: compile failure because `DefaultConfig` does not exist.

- [ ] **Step 3: Implement config object**

Create `internal/spinningwheel/config.go`:

```go
package spinningwheel

import (
	"time"

	"github.com/genie-cvc/verdandi/internal/verdandi"
)

type Config struct {
	Name               string        `json:"name"`
	Enabled            bool          `json:"enabled"`
	DataDir            string        `json:"dataDir"`
	Addr               string        `json:"addr"`
	StreamPollInterval time.Duration `json:"streamPollInterval"`
}

func DefaultConfig() Config {
	return Config{
		Name:               "Spinning Wheel",
		Enabled:            true,
		DataDir:            verdandi.DefaultDataDir(),
		Addr:               "127.0.0.1:8787",
		StreamPollInterval: 750 * time.Millisecond,
	}
}

func (c Config) WithDataDir(dataDir string) Config {
	if dataDir != "" {
		c.DataDir = dataDir
	}
	return c
}

func (c Config) WithAddr(addr string) Config {
	if addr != "" {
		c.Addr = addr
	}
	return c
}

func (c Config) WithEnabled(enabled bool) Config {
	c.Enabled = enabled
	return c
}
```

- [ ] **Step 4: Update command startup**

Modify `cmd/verdandi-spinning-wheel/main.go` to build a `spinningwheel.Config`:

```go
config := spinningwheel.DefaultConfig().
	WithDataDir(*dataDir).
	WithAddr(*addr).
	WithEnabled(true)

server := spinningwheel.NewServer(config.DataDir)
fmt.Fprintf(os.Stderr, "%s listening on http://%s\n", config.Name, config.Addr)
if err := http.ListenAndServe(config.Addr, server.Handler()); err != nil {
	fmt.Fprintf(os.Stderr, "spinning wheel server error: %v\n", err)
	os.Exit(1)
}
```

- [ ] **Step 5: Verify and commit**

Run:

```bash
go test ./internal/spinningwheel -run 'TestDefaultConfig|TestConfigWithOverrides' -count=1
go test ./...
git diff --check
```

Commit:

```bash
git add cmd/verdandi-spinning-wheel/main.go internal/spinningwheel/config.go internal/spinningwheel/config_test.go
git commit -m "feat: add spinning wheel plugin config"
```

## Task 2: Add Plugin Status and Health APIs

**Files:**
- Modify: `internal/spinningwheel/server.go`
- Modify: `internal/spinningwheel/server_test.go`

- [ ] **Step 1: Write failing API tests**

Add tests to `internal/spinningwheel/server_test.go`:

```go
func TestServerReturnsPluginStatus(t *testing.T) {
	server := httptest.NewServer(NewServer(t.TempDir()).Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/api/plugin")
	if err != nil {
		t.Fatalf("get plugin: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from plugin status, got %s", response.Status)
	}

	var payload struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Status  string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode plugin status: %v", err)
	}
	if payload.Name != "Spinning Wheel" || !payload.Enabled || payload.Status != "ready" {
		t.Fatalf("unexpected plugin status: %#v", payload)
	}
}

func TestServerHealth(t *testing.T) {
	server := httptest.NewServer(NewServer(t.TempDir()).Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/api/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from health, got %s", response.Status)
	}

	var payload struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if !payload.OK {
		t.Fatalf("expected healthy server")
	}
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
go test ./internal/spinningwheel -run 'TestServerReturnsPluginStatus|TestServerHealth' -count=1
```

Expected: 404 responses.

- [ ] **Step 3: Add routes and handlers**

Modify `internal/spinningwheel/server.go`:

```go
func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/plugin", s.handlePlugin)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/runs/", s.handleRunEvents)
	// existing static file handler remains unchanged
}

func (s Server) handlePlugin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"name":    "Spinning Wheel",
		"enabled": true,
		"status":  "ready",
	})
}

func (s Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
```

- [ ] **Step 4: Verify and commit**

Run:

```bash
go test ./internal/spinningwheel -run 'TestServerReturnsPluginStatus|TestServerHealth' -count=1
go test ./...
git diff --check
```

Commit:

```bash
git add internal/spinningwheel/server.go internal/spinningwheel/server_test.go
git commit -m "feat: expose spinning wheel plugin status"
```

## Task 3: Harden Live Streaming With Heartbeats and Cursor Resume

**Files:**
- Modify: `internal/spinningwheel/server.go`
- Modify: `internal/spinningwheel/server_test.go`
- Modify: `internal/spinningwheel/web/app.js`

- [ ] **Step 1: Write failing SSE tests**

Add these tests to `internal/spinningwheel/server_test.go`:

```go
func TestServerStreamsHeartbeatWhenFollowing(t *testing.T) {
	dataDir := t.TempDir()
	runID := "run_heartbeat_test"
	if err := verdandi.NewEventStoreForDataDir(dataDir).Reset(runID); err != nil {
		t.Fatalf("reset events: %v", err)
	}

	server := httptest.NewServer(NewServer(dataDir).Handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/runs/"+runID+"/events/stream?follow=1", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("get stream: %v", err)
	}
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), ": heartbeat") {
			return
		}
	}
	t.Fatalf("expected heartbeat comment from stream")
}

func TestServerStreamsEventsAfterCursor(t *testing.T) {
	dataDir := t.TempDir()
	runID := "run_cursor_test"
	store := verdandi.NewEventStoreForDataDir(dataDir)
	if err := store.Save(runID, []verdandi.VisualizationEvent{
		{RunID: runID, Type: verdandi.EventRunStarted, Timestamp: time.Unix(1, 0).UTC()},
		{RunID: runID, Type: verdandi.EventRunCompleted, Timestamp: time.Unix(2, 0).UTC()},
	}); err != nil {
		t.Fatalf("save events: %v", err)
	}

	server := httptest.NewServer(NewServer(dataDir).Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/api/runs/" + runID + "/events/stream?cursor=1")
	if err != nil {
		t.Fatalf("get stream: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	stream := string(body)
	if strings.Contains(stream, `"type":"run-started"`) {
		t.Fatalf("expected cursor to skip first event, got %s", stream)
	}
	if !strings.Contains(stream, `"type":"run-completed"`) {
		t.Fatalf("expected cursor to include second event, got %s", stream)
	}
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
go test ./internal/spinningwheel -run 'TestServerStreamsHeartbeatWhenFollowing|TestServerStreamsEventsAfterCursor' -count=1
```

Expected: heartbeat test fails because no heartbeat is written; cursor test fails because all historical events are streamed.

- [ ] **Step 3: Implement cursor and heartbeat**

Modify `streamRunEvents` in `internal/spinningwheel/server.go`:

```go
cursor := parseCursor(r.URL.Query().Get("cursor"), len(events))
for _, event := range events[cursor:] {
	if err := writeSSE(w, event); err != nil {
		return
	}
	flusher.Flush()
}
seen := len(events)
```

Add helpers:

```go
func parseCursor(raw string, max int) int {
	if raw == "" {
		return 0
	}
	cursor, err := strconv.Atoi(raw)
	if err != nil || cursor < 0 {
		return 0
	}
	if cursor > max {
		return max
	}
	return cursor
}

func writeHeartbeat(w http.ResponseWriter) error {
	_, err := w.Write([]byte(": heartbeat\n\n"))
	return err
}
```

Inside the follow ticker branch, write a heartbeat when no new events arrived:

```go
if len(latest) == seen {
	if err := writeHeartbeat(w); err != nil {
		return
	}
	flusher.Flush()
	continue
}
```

- [ ] **Step 4: Update browser stream cursor**

Modify `internal/spinningwheel/web/app.js` inside `connectLiveStream`:

```js
const cursor = events.length;
liveStream = new EventSource(`/api/runs/${encodeURIComponent(runId)}/events/stream?follow=1&cursor=${cursor}`);
```

- [ ] **Step 5: Verify and commit**

Run:

```bash
go test ./internal/spinningwheel -run 'TestServerStreamsHeartbeatWhenFollowing|TestServerStreamsEventsAfterCursor' -count=1
node --check internal/spinningwheel/web/app.js
go test ./...
git diff --check
```

Commit:

```bash
git add internal/spinningwheel/server.go internal/spinningwheel/server_test.go internal/spinningwheel/web/app.js
git commit -m "feat: harden spinning wheel live stream"
```

## Task 4: Add UI Focus and Filtering Controls

**Files:**
- Modify: `internal/spinningwheel/server_test.go`
- Modify: `internal/spinningwheel/web/index.html`
- Modify: `internal/spinningwheel/web/styles.css`
- Modify: `internal/spinningwheel/web/app.js`

- [ ] **Step 1: Write failing UI asset contract test**

Extend `TestServerServesSpinningWheelUI` in `internal/spinningwheel/server_test.go`:

```go
for _, expected := range []string{"stageFilter", "focusModeButton", "clearFocusButton"} {
	if !strings.Contains(string(body), expected) {
		t.Fatalf("expected UI shell to contain %q", expected)
	}
}

for _, expected := range []string{"applyStageFilter", "focusedAgentName", "setFocusedAgent"} {
	if !strings.Contains(app, expected) {
		t.Fatalf("expected app asset to contain %q", expected)
	}
}
```

- [ ] **Step 2: Run failing test**

Run:

```bash
go test ./internal/spinningwheel -run TestServerServesSpinningWheelUI -count=1
```

Expected: fail because the new controls and functions do not exist.

- [ ] **Step 3: Add controls to HTML**

Modify `internal/spinningwheel/web/index.html` near playback controls:

```html
<label class="filter-control" for="stageFilter">
  <span>Stage</span>
  <select id="stageFilter" aria-label="Stage filter">
    <option value="">All stages</option>
    <option value="planner">planner</option>
    <option value="code-writer">code-writer</option>
    <option value="tester">tester</option>
    <option value="documenter">documenter</option>
    <option value="deployer">deployer</option>
  </select>
</label>
<div class="controls">
  <button id="focusModeButton" type="button">Focus</button>
  <button id="clearFocusButton" type="button">Clear</button>
</div>
```

- [ ] **Step 4: Add filter state and behavior**

Modify `internal/spinningwheel/web/app.js`:

```js
const stageFilter = document.getElementById("stageFilter");
const focusModeButton = document.getElementById("focusModeButton");
const clearFocusButton = document.getElementById("clearFocusButton");
let selectedStage = "";
let focusedAgentName = "";

function applyStageFilter(event) {
  if (!selectedStage) {
    return true;
  }
  return event.stage === selectedStage || !event.stage;
}

function setFocusedAgent(name) {
  focusedAgentName = name || "";
  draw(performance.now());
  renderAgentRoster();
}
```

Update `appendTimeline`, `appendConversation`, and `renderAgentRoster` so filtered-out stage events are not shown in the timeline/conversation and focused agents get a visible class:

```js
if (!applyStageFilter(event)) {
  return;
}
```

Add listeners:

```js
stageFilter.addEventListener("change", () => {
  selectedStage = stageFilter.value;
  timeline.innerHTML = "";
  conversationItems = [];
  events.slice(0, replayIndex).filter(applyStageFilter).forEach((event) => {
    appendTimeline(event);
    appendConversation(event, event.agent ? agents.get(event.agent.name) : null);
  });
});

focusModeButton.addEventListener("click", () => setFocusedAgent(activeAgentName));
clearFocusButton.addEventListener("click", () => setFocusedAgent(""));
```

- [ ] **Step 5: Add styles**

Modify `internal/spinningwheel/web/styles.css`:

```css
.filter-control {
  display: grid;
  gap: 8px;
  color: #38443b;
  font-size: 13px;
  font-weight: 700;
}

.agent-roster li.is-focused {
  border-color: #5c6ac4;
  box-shadow: inset 4px 0 0 #5c6ac4;
}
```

- [ ] **Step 6: Verify and commit**

Run:

```bash
go test ./internal/spinningwheel -run TestServerServesSpinningWheelUI -count=1
node --check internal/spinningwheel/web/app.js
go test ./...
git diff --check
```

Commit:

```bash
git add internal/spinningwheel/server_test.go internal/spinningwheel/web/index.html internal/spinningwheel/web/styles.css internal/spinningwheel/web/app.js
git commit -m "feat: add spinning wheel focus filters"
```

## Task 5: Add Operator Documentation

**Files:**
- Create: `docs/spinning-wheel.md`
- Modify: `README.md`

- [ ] **Step 1: Write documentation**

Create `docs/spinning-wheel.md`:

````markdown
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
````

- [ ] **Step 2: Link from README**

Modify `README.md` by adding a short introduction link:

```markdown
## Spinning Wheel

Spinning Wheel is an optional visualizer for Verdandi runs. It shows agent creation, movement, decisions, speech bubbles, and live event streaming. See [docs/spinning-wheel.md](docs/spinning-wheel.md) for setup and operation.
```

- [ ] **Step 3: Verify docs and commit**

Run:

```bash
go test ./...
git diff --check
```

Commit:

```bash
git add README.md docs/spinning-wheel.md
git commit -m "docs: document spinning wheel plugin"
```

## Task 6: Final Integration Check

**Files:**
- No required file edits unless verification finds a defect.

- [ ] **Step 1: Run full test suite**

Run:

```bash
go test ./...
node --check internal/spinningwheel/web/app.js
git diff --check
```

Expected:
- All Go packages pass.
- `node --check` exits 0 with no output.
- `git diff --check` exits 0 with no output.

- [ ] **Step 2: Run local smoke test**

Start the server:

```bash
go run ./cmd/verdandi-spinning-wheel --data-dir .verdandi --addr 127.0.0.1:8787
```

In another terminal:

```bash
curl -fsS http://127.0.0.1:8787/api/health
curl -fsS http://127.0.0.1:8787/api/plugin
curl -fsS http://127.0.0.1:8787/ | rg 'Spinning Wheel|liveStatus|stageFilter'
```

Expected:
- `/api/health` returns `{"ok":true}`.
- `/api/plugin` returns `Spinning Wheel`, `enabled: true`, and `status: ready`.
- HTML contains `Spinning Wheel`, `liveStatus`, and `stageFilter`.

- [ ] **Step 3: Merge branch to develop**

Run:

```bash
git switch develop
git merge --no-ff feature/spinning-wheel-plugin-hardening -m "merge: spinning wheel plugin hardening"
go test ./...
```

Expected: merge succeeds and tests pass on `develop`.

## Self-Review

- Spec coverage: The plan covers plugin configuration, plugin status/health APIs, live streaming hardening, UI focus/filter controls, documentation, and final integration.
- Placeholder scan: The plan contains no unfinished markers and no unspecified "add tests" step without concrete test code.
- Type consistency: The plan uses existing `verdandi.VisualizationEvent`, `EventStore.Reset`, `EventStore.Save`, `EventStore.Append`, `NewServer`, and existing web asset IDs consistently.
