package verdandi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowPackageWritesHandoffAndAssetsWithoutAppSource(t *testing.T) {
	outputDir := t.TempDir()
	pkg := WorkflowPackage{
		RunID:   "run_test",
		Request: "계산기 CLI를 구현하고 테스트해줘",
		Agents: []AgentAsset{{
			ID:      "agent:go-cli-implementer:v1",
			Name:    "GoCliImplementer",
			Role:    "code-writer",
			Version: 1,
			Status:  AssetStatusActive,
			Contract: AgentContract{
				Name:        "GoCliImplementer",
				Description: "Implements Go CLI features.",
				Command:     "codex",
				Spec: AgentSpec{
					Role:         "code-writer",
					Capabilities: []string{"go", "cli", "tdd"},
				},
			},
		}},
		Skills: []SkillAsset{{
			ID:      "skill:go-cli-tdd:v1",
			Name:    "go-cli-tdd",
			Version: 1,
			Status:  AssetStatusActive,
			Contract: SkillContract{
				Name:         "go-cli-tdd",
				Description:  "Use tests first for Go CLI work.",
				WhenToUse:    "Go CLI feature work",
				Instructions: "Write failing tests, implement, run go test ./...",
				Inputs:       []string{"request"},
				Outputs:      []string{"patch", "testResults"},
				Constraints:  []string{"Verdandi does not write application code."},
			},
		}},
		Tasks: []WorkflowTask{{
			ID:          "implement",
			Title:       "Implement calculator CLI",
			AgentID:     "agent:go-cli-implementer:v1",
			SkillIDs:    []string{"skill:go-cli-tdd:v1"},
			DependsOn:   []string{},
			Description: "External coding agent implements the requested CLI.",
		}},
	}

	written, err := WriteWorkflowPackage(outputDir, pkg)
	if err != nil {
		t.Fatalf("WriteWorkflowPackage returned error: %v", err)
	}

	for _, name := range []string{"workflow.json", "handoff.md", "selected-assets.json"} {
		if _, err := os.Stat(filepath.Join(written.OutputDir, name)); err != nil {
			t.Fatalf("expected %s to be written: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(written.OutputDir, "agents", "agent-go-cli-implementer-v1.json")); err != nil {
		t.Fatalf("expected agent asset file to be written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(written.OutputDir, "skills", "skill-go-cli-tdd-v1.md")); err != nil {
		t.Fatalf("expected skill asset file to be written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(written.OutputDir, "main.go")); !os.IsNotExist(err) {
		t.Fatalf("workflow package must not create app source code, stat err = %v", err)
	}
	handoff, err := os.ReadFile(filepath.Join(written.OutputDir, "handoff.md"))
	if err != nil {
		t.Fatalf("read handoff: %v", err)
	}
	text := string(handoff)
	if !strings.Contains(text, "계산기 CLI를 구현하고 테스트해줘") {
		t.Fatalf("handoff missing original request: %s", text)
	}
	if !strings.Contains(text, "Verdandi does not write application code") {
		t.Fatalf("handoff missing execution boundary: %s", text)
	}
}

func TestWorkflowPackageUsesSafeAssetFilenames(t *testing.T) {
	outputDir := t.TempDir()
	pkg := WorkflowPackage{
		RunID:   "run_safe_names",
		Request: "safe filenames",
		Agents: []AgentAsset{{
			ID:      "agent:../go/cli:v1",
			Name:    "GoCliImplementer",
			Role:    "code-writer",
			Version: 1,
			Status:  AssetStatusActive,
			Contract: AgentContract{
				Name: "GoCliImplementer",
				Spec: AgentSpec{Role: "code-writer"},
			},
		}},
		Skills: []SkillAsset{{
			ID:      "skill:../go/cli:v1",
			Name:    "go-cli-tdd",
			Version: 1,
			Status:  AssetStatusActive,
			Contract: SkillContract{
				Name:         "go-cli-tdd",
				Description:  "Use tests first.",
				WhenToUse:    "Go CLI work",
				Instructions: "Test first.",
			},
		}},
	}

	written, err := WriteWorkflowPackage(outputDir, pkg)
	if err != nil {
		t.Fatalf("WriteWorkflowPackage returned error: %v", err)
	}

	agentDir := filepath.Join(written.OutputDir, "agents")
	skillDir := filepath.Join(written.OutputDir, "skills")
	if _, err := os.Stat(filepath.Join(agentDir, "agent-go-cli-v1.json")); err != nil {
		t.Fatalf("expected sanitized agent filename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "skill-go-cli-v1.md")); err != nil {
		t.Fatalf("expected sanitized skill filename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(written.OutputDir, "go")); !os.IsNotExist(err) {
		t.Fatalf("asset ID should not create nested workflow paths, stat err = %v", err)
	}
}

func TestWorkflowPackageReturnsJSONSerializationError(t *testing.T) {
	outputDir := t.TempDir()
	pkg := WorkflowPackage{
		RunID:   "run_bad_json",
		Request: "bad json",
		Agents: []AgentAsset{{
			ID:      "agent:bad-json:v1",
			Name:    "BadJSONAgent",
			Role:    "code-writer",
			Version: 1,
			Status:  AssetStatusActive,
			Contract: AgentContract{
				Name:     "BadJSONAgent",
				Metadata: map[string]any{"cannotMarshal": func() {}},
			},
		}},
	}

	written, err := WriteWorkflowPackage(outputDir, pkg)
	if err == nil {
		t.Fatalf("expected JSON serialization error, got result %#v", written)
	}
	if _, statErr := os.Stat(filepath.Join(outputDir, "workflows", "run_bad_json", "workflow.json")); !os.IsNotExist(statErr) {
		t.Fatalf("workflow.json should not be written after JSON serialization error, stat err = %v", statErr)
	}
}
