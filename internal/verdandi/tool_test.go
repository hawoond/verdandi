package verdandi

import (
	"path/filepath"
	"testing"
)

func TestToolRunAcceptsNaturalLanguageRequest(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	result, err := tool.Handle("run", map[string]any{
		"request": "계산기 앱을 기획하고 구현하고 테스트해줘",
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if result["ok"] != true {
		t.Fatalf("expected ok result, got %#v", result)
	}
	if result["action"] != "run" {
		t.Fatalf("expected run action, got %#v", result["action"])
	}
	if result["runId"] == "" {
		t.Fatalf("expected runId, got %#v", result)
	}
	if result["request"] != "계산기 앱을 기획하고 구현하고 테스트해줘" {
		t.Fatalf("request was not echoed: %#v", result)
	}
}

func TestToolAnalyzeReturnsExecutionPlan(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	result, err := tool.Analyze("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if result["action"] != "analyze" {
		t.Fatalf("expected analyze action, got %#v", result["action"])
	}

	plan, ok := result["plan"].(Plan)
	if !ok {
		t.Fatalf("expected typed Plan in analyze result, got %#v", result["plan"])
	}
	if plan.StageCount != 4 {
		t.Fatalf("expected 4 stages, got %#v", plan)
	}

	got := []string{}
	for _, stage := range plan.Stages {
		got = append(got, stage.Stage)
	}
	want := []string{"planner", "code-writer", "tester", "documenter"}
	if !equalStrings(got, want) {
		t.Fatalf("stages mismatch: got %#v want %#v", got, want)
	}
}

func TestToolUsesConfiguredAnalyzer(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{
		DataDir:  dataDir,
		Analyzer: AnalyzerKeyword,
	})

	result, err := tool.Analyze("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if result["intent"] != IntentPlanner {
		t.Fatalf("expected planner intent, got %#v", result["intent"])
	}
	plan := result["plan"].(Plan)
	if plan.StageCount != 4 {
		t.Fatalf("expected 4 stages, got %#v", plan)
	}
}

func TestToolStoresStatusAndListsOutput(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	result, err := tool.Run("문서 작성")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	runID := result["runId"].(string)
	status, err := tool.GetStatus(runID)
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	if status["status"] != "success" {
		t.Fatalf("expected success status, got %#v", status["status"])
	}

	opened, err := tool.OpenOutput(runID)
	if err != nil {
		t.Fatalf("open output failed: %v", err)
	}
	files := opened["files"].([]FileInfo)
	if len(files) == 0 {
		t.Fatalf("expected output files")
	}
	for _, file := range files {
		if filepath.IsAbs(file.Name) {
			t.Fatalf("file name should not be absolute: %#v", file)
		}
	}
}

func TestToolStatusIncludesTypedStageResults(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	result, err := tool.Run("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	status, err := tool.GetStatus(result["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	stages := status["stages"].([]StageResult)
	if len(stages) != 4 {
		t.Fatalf("expected 4 stages, got %#v", stages)
	}
	for _, stage := range stages {
		if stage.Result == nil {
			t.Fatalf("expected result for stage %s", stage.Stage)
		}
	}
}
