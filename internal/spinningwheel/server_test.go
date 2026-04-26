package spinningwheel

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/genie-cvc/verdandi/internal/verdandi"
)

func TestServerListsRunsAndEvents(t *testing.T) {
	dataDir := t.TempDir()
	runID := "run_spinning_wheel_test"
	record := verdandi.RunRecord{
		RunID:       runID,
		Status:      "success",
		Request:     "시각화 테스트",
		Summary:     verdandi.Summary{TotalStages: 1, Success: 1, Files: []verdandi.FileEntry{}},
		CreatedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
		Stages: []verdandi.StageResult{{
			Stage:  "planner",
			Status: "success",
			Agent: &verdandi.AgentContract{
				Name: "PlannerCat",
				Spec: verdandi.AgentSpec{Role: "planner", Capabilities: []string{"planning"}},
			},
			Started: time.Now().UTC(),
			Ended:   time.Now().UTC(),
		}},
	}
	if err := verdandi.NewStoreForDataDir(dataDir).Save(record); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := verdandi.NewEventStoreForDataDir(dataDir).SaveRun(record); err != nil {
		t.Fatalf("save events: %v", err)
	}

	server := httptest.NewServer(NewServer(dataDir).Handler())
	defer server.Close()

	runsResponse, err := http.Get(server.URL + "/api/runs")
	if err != nil {
		t.Fatalf("get runs: %v", err)
	}
	defer runsResponse.Body.Close()
	if runsResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from runs, got %s", runsResponse.Status)
	}
	var runsPayload struct {
		Runs []verdandi.RunRecord `json:"runs"`
	}
	if err := json.NewDecoder(runsResponse.Body).Decode(&runsPayload); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runsPayload.Runs) != 1 || runsPayload.Runs[0].RunID != runID {
		t.Fatalf("expected saved run, got %#v", runsPayload)
	}

	eventsResponse, err := http.Get(server.URL + "/api/runs/" + runID + "/events")
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	defer eventsResponse.Body.Close()
	if eventsResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from events, got %s", eventsResponse.Status)
	}
	var eventsPayload struct {
		Events []verdandi.VisualizationEvent `json:"events"`
	}
	if err := json.NewDecoder(eventsResponse.Body).Decode(&eventsPayload); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(eventsPayload.Events) == 0 {
		t.Fatalf("expected visualization events")
	}
	if eventsPayload.Events[0].RunID != runID {
		t.Fatalf("expected event runId %s, got %#v", runID, eventsPayload.Events[0])
	}
}

func TestServerStreamsRunEvents(t *testing.T) {
	dataDir := t.TempDir()
	runID := "run_spinning_wheel_stream_test"
	record := verdandi.RunRecord{
		RunID:       runID,
		Status:      "success",
		Request:     "스트림 테스트",
		Summary:     verdandi.Summary{TotalStages: 1, Success: 1, Files: []verdandi.FileEntry{}},
		CreatedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
		Stages: []verdandi.StageResult{{
			Stage:  "planner",
			Status: "success",
			Agent: &verdandi.AgentContract{
				Name: "PlannerCat",
				Spec: verdandi.AgentSpec{Role: "planner", Capabilities: []string{"planning"}},
			},
			Started: time.Now().UTC(),
			Ended:   time.Now().UTC(),
		}},
	}
	if err := verdandi.NewEventStoreForDataDir(dataDir).SaveRun(record); err != nil {
		t.Fatalf("save events: %v", err)
	}

	server := httptest.NewServer(NewServer(dataDir).Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/api/runs/" + runID + "/events/stream")
	if err != nil {
		t.Fatalf("get event stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from event stream, got %s", response.Status)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("expected event stream content type, got %q", contentType)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	stream := string(body)
	if !strings.Contains(stream, "event: visualization-event") {
		t.Fatalf("expected named visualization events, got %s", stream)
	}
	if !strings.Contains(stream, `"runId":"`+runID+`"`) {
		t.Fatalf("expected stream to contain run ID %s, got %s", runID, stream)
	}
}

func TestServerStreamsEventsAppendedAfterConnection(t *testing.T) {
	dataDir := t.TempDir()
	runID := "run_spinning_wheel_live_test"
	events := verdandi.NewEventStoreForDataDir(dataDir)
	if err := events.Reset(runID); err != nil {
		t.Fatalf("reset events: %v", err)
	}

	server := httptest.NewServer(NewServer(dataDir).Handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/runs/"+runID+"/events/stream?follow=1", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("get live stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from live stream, got %s", response.Status)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = events.Append(verdandi.VisualizationEvent{
			RunID:     runID,
			Type:      verdandi.EventRunStarted,
			Timestamp: time.Now().UTC(),
			Status:    "running",
			Message:   "live request",
		})
	}()

	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `"runId":"`+runID+`"`) && strings.Contains(line, `"type":"run-started"`) {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan live stream: %v", err)
	}
	t.Fatalf("expected live stream to include appended run-started event")
}

func TestServerServesSpinningWheelUI(t *testing.T) {
	server := httptest.NewServer(NewServer(t.TempDir()).Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get root: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from root, got %s", response.Status)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "Spinning Wheel") {
		t.Fatalf("expected Spinning Wheel UI, got %s", string(body))
	}
	for _, expected := range []string{"playPauseButton", "stepButton", "speedRange", "eventCounter", "agentRoster", "conversationLog", "liveStatus"} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("expected UI shell to contain %q", expected)
		}
	}

	assetResponse, err := http.Get(server.URL + "/app.js")
	if err != nil {
		t.Fatalf("get app asset: %v", err)
	}
	defer assetResponse.Body.Close()
	if assetResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from app asset, got %s", assetResponse.Status)
	}
	assetBody, err := io.ReadAll(assetResponse.Body)
	if err != nil {
		t.Fatalf("read app asset: %v", err)
	}
	app := string(assetBody)
	for _, expected := range []string{"requestAnimationFrame", "moveAgentTo", "spawnedAt", "agent-spawned", "speechForEvent", "drawDecisionLinks", "activeStage", "stepForward", "playbackDelay", "renderAgentRoster", "appendConversation", "connectLiveStream", "EventSource"} {
		if !strings.Contains(app, expected) {
			t.Fatalf("expected app asset to contain %q", expected)
		}
	}
}
