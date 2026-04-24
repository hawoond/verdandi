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
