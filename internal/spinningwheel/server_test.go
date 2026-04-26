package spinningwheel

import (
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
	for _, expected := range []string{"playPauseButton", "stepButton", "speedRange", "eventCounter"} {
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
	for _, expected := range []string{"requestAnimationFrame", "moveAgentTo", "spawnedAt", "agent-spawned", "speechForEvent", "drawDecisionLinks", "activeStage", "stepForward", "playbackDelay"} {
		if !strings.Contains(app, expected) {
			t.Fatalf("expected app asset to contain %q", expected)
		}
	}
}
