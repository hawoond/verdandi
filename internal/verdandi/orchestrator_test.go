package verdandi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRequestBuildsExecutionGraph(t *testing.T) {
	plan := NewOrchestrator("").ParseRequest("기획 구현 테스트 문서화")

	nodes := []string{}
	for _, stage := range plan.Stages {
		nodes = append(nodes, stage.Stage)
	}

	wantNodes := []string{"planner", "code-writer", "tester", "documenter"}
	if !equalStrings(nodes, wantNodes) {
		t.Fatalf("nodes mismatch: got %#v want %#v", nodes, wantNodes)
	}

	if !hasEdge(plan.Graph.Edges, "planner", "code-writer") {
		t.Fatalf("missing planner -> code-writer edge: %#v", plan.Graph.Edges)
	}
	if !hasEdge(plan.Graph.Edges, "code-writer", "tester") {
		t.Fatalf("missing code-writer -> tester edge: %#v", plan.Graph.Edges)
	}
	if !hasEdge(plan.Graph.Edges, "tester", "documenter") {
		t.Fatalf("missing tester -> documenter edge: %#v", plan.Graph.Edges)
	}
}

func TestCodeWriterAutomaticallyAddsTester(t *testing.T) {
	plan := NewOrchestrator("").ParseRequest("간단한 앱 구현")

	if !containsStage(plan.Stages, "tester") {
		t.Fatalf("expected tester stage to be added: %#v", plan.Stages)
	}
}

func TestExecuteReturnsTypedStageOutputs(t *testing.T) {
	dataDir := t.TempDir()
	result, err := NewOrchestrator(dataDir).Execute("기획 구현 테스트 문서화", nil)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(result.Stages) != 4 {
		t.Fatalf("expected 4 stages, got %#v", result.Stages)
	}
	for _, stage := range result.Stages {
		if stage.Status != "success" {
			t.Fatalf("expected success stage, got %#v", stage)
		}
		if stage.Result == nil {
			t.Fatalf("expected typed result for stage %s", stage.Stage)
		}
	}

	if result.Stages[0].Result.Type != "files" {
		t.Fatalf("expected planner files output, got %#v", result.Stages[0].Result)
	}
	if result.Summary.TotalStages != 4 || result.Summary.Success != 4 || result.Summary.Failed != 0 {
		t.Fatalf("unexpected summary: %#v", result.Summary)
	}
	if len(result.Summary.Files) == 0 {
		t.Fatalf("expected generated files in summary")
	}
}

func TestExecuteGeneratesRunnableGoProjectArtifacts(t *testing.T) {
	dataDir := t.TempDir()
	result, err := NewOrchestrator(dataDir).Execute("계산기 앱을 기획하고 구현하고 테스트하고 문서화해줘", nil)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	wantFiles := map[string]bool{
		"requirements.md": false,
		"go.mod":          false,
		"main.go":         false,
		"main_test.go":    false,
		"README.md":       false,
	}
	for _, file := range result.Summary.Files {
		if _, ok := wantFiles[file.Name]; ok {
			wantFiles[file.Name] = true
		}
	}
	for name, found := range wantFiles {
		if !found {
			t.Fatalf("expected generated file %s in %#v", name, result.Summary.Files)
		}
	}

	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(result.OutputDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(data)
	}

	requirements := read("requirements.md")
	for _, want := range []string{"# Requirements", "## Goal", "## Workflow", "## Acceptance Criteria"} {
		if !strings.Contains(requirements, want) {
			t.Fatalf("requirements missing %q:\n%s", want, requirements)
		}
	}

	mainFile := read("main.go")
	for _, want := range []string{"package main", "func main()", "Verdandi generated project"} {
		if !strings.Contains(mainFile, want) {
			t.Fatalf("main.go missing %q:\n%s", want, mainFile)
		}
	}

	readme := read("README.md")
	for _, want := range []string{"# Generated Project", "## Request", "## Files", "go test ./..."} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README.md missing %q:\n%s", want, readme)
		}
	}
}

func TestRunValidationRunsGoTest(t *testing.T) {
	outputDir := t.TempDir()
	writeFile(t, outputDir, "go.mod", "module generated-project\n\ngo 1.22\n")
	writeFile(t, outputDir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, outputDir, "main_test.go", "package main\n\nimport \"testing\"\n\nfunc TestPass(t *testing.T) {}\n")

	result, err := runValidation(outputDir)
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	if result.Type != "test" || result.Status != "success" {
		t.Fatalf("unexpected validation output: %#v", result)
	}
	if len(result.Tests) != 1 || result.Tests[0].Name != "go-test" || result.Tests[0].Status != "success" {
		t.Fatalf("unexpected test results: %#v", result.Tests)
	}
}

func TestRunValidationReportsGoTestFailure(t *testing.T) {
	outputDir := t.TempDir()
	writeFile(t, outputDir, "go.mod", "module generated-project\n\ngo 1.22\n")
	writeFile(t, outputDir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, outputDir, "main_test.go", "package main\n\nimport \"testing\"\n\nfunc TestFail(t *testing.T) { t.Fatal(\"boom\") }\n")

	_, err := runValidation(outputDir)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "go test failed") {
		t.Fatalf("expected go test failure message, got %v", err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasEdge(edges []Edge, from, to string) bool {
	for _, edge := range edges {
		if edge.From == from && edge.To == to {
			return true
		}
	}
	return false
}

func containsStage(stages []StageDef, name string) bool {
	for _, stage := range stages {
		if stage.Stage == name {
			return true
		}
	}
	return false
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
