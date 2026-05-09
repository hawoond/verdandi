package verdandi

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type staticAnalyzer struct {
	result AnalysisResult
	err    error
}

func (a staticAnalyzer) Analyze(request string) (AnalysisResult, error) {
	if a.err != nil {
		return AnalysisResult{}, a.err
	}
	a.result.Text = request
	return a.result, nil
}

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

func TestToolRunRejectsEmptyRequest(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	_, err := tool.Run("   ")
	if err == nil {
		t.Fatal("expected empty request to be rejected")
	}
	if !strings.Contains(err.Error(), "request") {
		t.Fatalf("expected request validation error, got %v", err)
	}
}

func TestToolRunPlanExecutesClientSelectedStagesWithoutAnalyzer(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{err: errors.New("analyzer should not be called")})

	result, err := tool.Handle("run_plan", map[string]any{
		"request": "build and test from client plan",
		"stages": []any{
			map[string]any{"stage": "code-writer", "keyword": "client-llm"},
		},
	})
	if err != nil {
		t.Fatalf("run_plan failed: %v", err)
	}

	if result["action"] != "run_plan" {
		t.Fatalf("expected run_plan action, got %#v", result["action"])
	}
	if result["analyzer"] != "client-plan" {
		t.Fatalf("expected client-plan analyzer, got %#v", result["analyzer"])
	}
	summary := result["summary"].(Summary)
	if summary.TotalStages != 2 || summary.Failed != 0 {
		t.Fatalf("expected code-writer plus auto tester to succeed, got %#v", summary)
	}
}

func TestToolRunPlanIncludesPlannerArtifacts(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	result, err := tool.Handle("run_plan", map[string]any{
		"request": "plan then build a calculator",
		"stages": []any{
			map[string]any{"stage": "planner", "keyword": "client-llm"},
			map[string]any{"stage": "code-writer", "keyword": "client-llm"},
		},
	})
	if err != nil {
		t.Fatalf("run_plan failed: %v", err)
	}

	summary := result["summary"].(Summary)
	wantFiles := map[string]bool{
		"requirements.md":        false,
		"acceptance-criteria.md": false,
		"task-breakdown.md":      false,
		"risks.md":               false,
	}
	for _, file := range summary.Files {
		if _, ok := wantFiles[file.Name]; ok {
			wantFiles[file.Name] = true
		}
	}
	for name, found := range wantFiles {
		if !found {
			t.Fatalf("expected planner artifact %s in %#v", name, summary.Files)
		}
	}
}

func TestToolValidatePlanNormalizesClientSelectedStages(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	result, err := tool.Handle("validate_plan", map[string]any{
		"request": "document after building",
		"stages": []any{
			map[string]any{"stage": "documenter", "keyword": "client-llm"},
			map[string]any{"stage": "code-writer", "keyword": "client-llm"},
		},
	})
	if err != nil {
		t.Fatalf("validate_plan failed: %v", err)
	}

	if result["action"] != "validate_plan" {
		t.Fatalf("expected validate_plan action, got %#v", result["action"])
	}
	plan := result["plan"].(Plan)
	got := []string{}
	for _, stage := range plan.Stages {
		got = append(got, stage.Stage)
	}
	want := []string{"code-writer", "tester", "documenter"}
	if !equalStrings(got, want) {
		t.Fatalf("stages mismatch: got %#v want %#v", got, want)
	}
}

func TestToolValidatePlanRejectsUnknownClientStage(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	_, err := tool.Handle("validate_plan", map[string]any{
		"request": "bad stage",
		"stages": []any{
			map[string]any{"stage": "shell", "keyword": "client-llm"},
		},
	})
	if err == nil {
		t.Fatal("expected unknown stage to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown stage") {
		t.Fatalf("expected unknown stage error, got %v", err)
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

func TestPrepareWorkflowCreatesPersistentReusableAssets(t *testing.T) {
	dataDir := t.TempDir()
	request := "접근성 좋은 계산기 앱을 기획하고 구현하고 테스트해줘"
	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan(request, []StageDef{
		{Stage: "planner", Keyword: "기획"},
		{Stage: "code-writer", Keyword: "구현"},
	})
	if err != nil {
		t.Fatalf("normalize plan: %v", err)
	}
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{
		result: AnalysisResult{
			Intent:     IntentResult{Category: IntentPlanner, Confidence: 0.92},
			Complexity: ComplexityResult{Level: "MEDIUM", Score: 5},
			Plan:       plan,
			Source:     AnalyzerKeyword,
		},
	})

	result, err := tool.PrepareWorkflow(request)
	if err != nil {
		t.Fatalf("PrepareWorkflow failed: %v", err)
	}

	if result["ok"] != true {
		t.Fatalf("expected ok result, got %#v", result)
	}
	if result["action"] != "prepare_workflow" {
		t.Fatalf("expected prepare_workflow action, got %#v", result["action"])
	}
	if result["status"] != "prepared" {
		t.Fatalf("expected prepared status, got %#v", result["status"])
	}
	runID, ok := result["runId"].(string)
	if !ok || runID == "" {
		t.Fatalf("expected runId, got %#v", result["runId"])
	}
	outputDir, ok := result["outputDir"].(string)
	if !ok || outputDir == "" {
		t.Fatalf("expected outputDir, got %#v", result["outputDir"])
	}
	if !strings.HasPrefix(outputDir, filepath.Join(dataDir, "workflows", runID)) {
		t.Fatalf("workflow outputDir %q is not under data dir run path", outputDir)
	}
	for _, name := range []string{"workflow.json", "handoff.md", "selected-assets.json"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected workflow file %s: %v", name, err)
		}
	}

	workflow, ok := result["workflow"].(WorkflowPackage)
	if !ok {
		t.Fatalf("expected typed WorkflowPackage, got %#v", result["workflow"])
	}
	if workflow.RunID != runID {
		t.Fatalf("workflow runID = %q, want %q", workflow.RunID, runID)
	}
	if len(workflow.Agents) == 0 || len(workflow.Skills) == 0 || len(workflow.Tasks) == 0 {
		t.Fatalf("expected reusable assets and tasks, got %#v", workflow)
	}
	assertWorkflowTasksReferenceAssets(t, workflow)

	registry := NewAssetRegistry(dataDir)
	agents, err := registry.ListAgents()
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	skills, err := registry.ListSkills()
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(agents) != len(workflow.Agents) {
		t.Fatalf("registry agents = %d, workflow agents = %d", len(agents), len(workflow.Agents))
	}
	if len(skills) != len(workflow.Skills) {
		t.Fatalf("registry skills = %d, workflow skills = %d", len(skills), len(workflow.Skills))
	}

	status, err := tool.GetStatus(runID)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status["status"] != "prepared" {
		t.Fatalf("stored status = %#v, want prepared", status["status"])
	}
	summary := result["summary"].(Summary)
	if summary.Success != 0 || summary.Failed != 0 {
		t.Fatalf("prepared summary should not report execution success/failure, got %#v", summary)
	}
}

func TestPrepareWorkflowReusesExistingAssetsWithoutActiveVersionChurn(t *testing.T) {
	dataDir := t.TempDir()
	request := "접근성 좋은 계산기 앱을 기획하고 구현하고 테스트해줘"
	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan(request, []StageDef{
		{Stage: "planner", Keyword: "기획"},
		{Stage: "code-writer", Keyword: "구현"},
	})
	if err != nil {
		t.Fatalf("normalize plan: %v", err)
	}
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{
		result: AnalysisResult{
			Intent:     IntentResult{Category: IntentPlanner, Confidence: 0.92},
			Complexity: ComplexityResult{Level: "MEDIUM", Score: 5},
			Plan:       plan,
			Source:     AnalyzerKeyword,
		},
	})

	first, err := tool.PrepareWorkflow(request)
	if err != nil {
		t.Fatalf("first PrepareWorkflow failed: %v", err)
	}
	second, err := tool.PrepareWorkflow(request)
	if err != nil {
		t.Fatalf("second PrepareWorkflow failed: %v", err)
	}

	firstWorkflow := first["workflow"].(WorkflowPackage)
	secondWorkflow := second["workflow"].(WorkflowPackage)
	assertWorkflowTasksReferenceAssets(t, firstWorkflow)
	assertWorkflowTasksReferenceAssets(t, secondWorkflow)

	registry := NewAssetRegistry(dataDir)
	agents, err := registry.ListAgents()
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) != len(secondWorkflow.Agents) {
		t.Fatalf("registry agents = %d, workflow agents = %d", len(agents), len(secondWorkflow.Agents))
	}
	activeByName := map[string]int{}
	for _, agent := range agents {
		if agent.Status == AssetStatusActive {
			activeByName[agent.Name]++
		}
	}
	for name, count := range activeByName {
		if count > 1 {
			t.Fatalf("agent %q has %d active versions; want at most 1", name, count)
		}
	}
	for index, agent := range firstWorkflow.Agents {
		if secondWorkflow.Agents[index].ID != agent.ID {
			t.Fatalf("agent %d ID churned: first %q second %q", index, agent.ID, secondWorkflow.Agents[index].ID)
		}
	}
}

func TestPrepareWorkflowDoesNotReuseNeedsReviewAgent(t *testing.T) {
	dataDir := t.TempDir()
	request := "계산기 앱을 구현하고 테스트해줘"
	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan(request, []StageDef{
		{Stage: "code-writer", Keyword: "구현"},
	})
	if err != nil {
		t.Fatalf("normalize plan: %v", err)
	}
	candidate := normalizeAgentAsset(assetAgentForStage(plan.Stages[0]), "")
	needsReview := candidate
	needsReview.Status = AssetStatusNeedsReview
	if err := NewAssetRegistry(dataDir).UpsertAgent(needsReview); err != nil {
		t.Fatalf("seed needs-review agent: %v", err)
	}
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{
		result: AnalysisResult{
			Intent: IntentResult{Category: IntentCodeWriter, Confidence: 0.9},
			Plan:   plan,
			Source: AnalyzerKeyword,
		},
	})

	result, err := tool.PrepareWorkflow(request)
	if err != nil {
		t.Fatalf("PrepareWorkflow failed: %v", err)
	}

	workflow := result["workflow"].(WorkflowPackage)
	var implementationAgent AgentAsset
	for _, agent := range workflow.Agents {
		if agent.Name == needsReview.Name {
			implementationAgent = agent
			break
		}
	}
	if implementationAgent.ID == "" {
		t.Fatalf("expected workflow to include implementation agent named %q: %#v", needsReview.Name, workflow.Agents)
	}
	if implementationAgent.ID == needsReview.ID {
		t.Fatalf("PrepareWorkflow reused needs-review agent %q", needsReview.ID)
	}
	if implementationAgent.Status != AssetStatusActive {
		t.Fatalf("new workflow agent status = %q, want active", implementationAgent.Status)
	}
}

func TestToolListSkillsReturnsPersistedSkillAssets(t *testing.T) {
	dataDir := t.TempDir()
	skill := SkillAsset{
		Name:    "go-cli-tdd",
		Version: 1,
		Status:  AssetStatusActive,
		Contract: SkillContract{
			Name:         "go-cli-tdd",
			Description:  "Guide Go CLI implementation with tests first.",
			WhenToUse:    "Use for Go command-line tools.",
			Instructions: "Write failing tests before implementation.",
			Inputs:       []string{"request"},
			Outputs:      []string{"patch"},
		},
	}
	if err := NewAssetRegistry(dataDir).UpsertSkill(skill); err != nil {
		t.Fatalf("seed skill: %v", err)
	}

	result, err := NewTool(Options{DataDir: dataDir}).Handle("list_skills", map[string]any{})
	if err != nil {
		t.Fatalf("list_skills failed: %v", err)
	}

	if result["ok"] != true || result["action"] != "list_skills" {
		t.Fatalf("unexpected result metadata: %#v", result)
	}
	if result["count"] != 1 {
		t.Fatalf("count = %#v, want 1", result["count"])
	}
	skills := result["skills"].([]SkillAsset)
	if len(skills) != 1 || skills[0].Name != "go-cli-tdd" {
		t.Fatalf("skills = %#v, want seeded skill", skills)
	}
}

func TestToolRecommendAssetsReturnsReusableAgentsAndSkills(t *testing.T) {
	dataDir := t.TempDir()
	registry := NewAssetRegistry(dataDir)
	for _, seed := range []struct {
		name   string
		status string
	}{
		{name: "GoCliImplementer", status: AssetStatusActive},
		{name: "NeedsReviewImplementer", status: AssetStatusNeedsReview},
		{name: "DeprecatedImplementer", status: AssetStatusDeprecated},
		{name: "ArchivedImplementer", status: AssetStatusArchived},
		{name: "SupersededImplementer", status: AssetStatusSuperseded},
	} {
		if err := registry.UpsertAgent(AgentAsset{
			Name:     seed.name,
			Role:     "code-writer",
			Version:  1,
			Status:   seed.status,
			Contract: AgentContract{Name: seed.name, Spec: AgentSpec{Role: "code-writer"}},
		}); err != nil {
			t.Fatalf("seed agent %s: %v", seed.name, err)
		}
	}
	for _, seed := range []struct {
		name   string
		status string
	}{
		{name: "go-cli-tdd", status: AssetStatusActive},
		{name: "needs-review-tdd", status: AssetStatusNeedsReview},
		{name: "deprecated-tdd", status: AssetStatusDeprecated},
		{name: "archived-tdd", status: AssetStatusArchived},
		{name: "superseded-tdd", status: AssetStatusSuperseded},
	} {
		if err := registry.UpsertSkill(SkillAsset{
			Name:     seed.name,
			Version:  1,
			Status:   seed.status,
			Contract: SkillContract{Name: seed.name},
		}); err != nil {
			t.Fatalf("seed skill %s: %v", seed.name, err)
		}
	}

	result, err := NewTool(Options{DataDir: dataDir}).Handle("recommend_assets", map[string]any{"request": "build a Go CLI with tests"})
	if err != nil {
		t.Fatalf("recommend_assets failed: %v", err)
	}

	if result["ok"] != true || result["action"] != "recommend_assets" {
		t.Fatalf("unexpected result metadata: %#v", result)
	}
	if result["request"] != "build a Go CLI with tests" {
		t.Fatalf("request was not echoed: %#v", result["request"])
	}
	agents := result["agents"].([]AgentAsset)
	skills := result["skills"].([]SkillAsset)
	if got := agentAssetNames(agents); !equalStrings(got, []string{"GoCliImplementer"}) {
		t.Fatalf("agents = %#v, want seeded agent", agents)
	}
	if got := skillAssetNames(skills); !equalStrings(got, []string{"go-cli-tdd"}) {
		t.Fatalf("skills = %#v, want seeded skill", skills)
	}
}

func TestToolRecordOutcomeUpdatesAssetMetrics(t *testing.T) {
	dataDir := t.TempDir()
	registry := NewAssetRegistry(dataDir)
	skill := SkillAsset{
		Name:     "go-cli-tdd",
		Version:  1,
		Status:   AssetStatusActive,
		Contract: SkillContract{Name: "go-cli-tdd"},
	}
	if err := registry.UpsertSkill(skill); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
	skills, err := registry.ListSkills()
	if err != nil {
		t.Fatalf("list seeded skills: %v", err)
	}

	result, err := NewTool(Options{DataDir: dataDir}).Handle("record_outcome", map[string]any{
		"assetId": skills[0].ID,
		"kind":    AssetKindSkill,
		"status":  "error",
		"runId":   "run_test",
		"error":   "missing assertion",
		"lesson":  "Add a focused regression test before reusing this skill.",
	})
	if err != nil {
		t.Fatalf("record_outcome failed: %v", err)
	}

	if result["ok"] != true || result["action"] != "record_outcome" {
		t.Fatalf("unexpected result metadata: %#v", result)
	}
	if result["assetId"] != skills[0].ID || result["kind"] != AssetKindSkill || result["status"] != "error" {
		t.Fatalf("unexpected outcome echo: %#v", result)
	}
	updated, err := registry.ListSkills()
	if err != nil {
		t.Fatalf("list updated skills: %v", err)
	}
	if updated[0].Metrics.TotalRuns != 1 || updated[0].Metrics.FailureRuns != 1 || updated[0].Metrics.LastError != "missing assertion" {
		t.Fatalf("metrics were not updated: %#v", updated[0].Metrics)
	}
	if len(updated[0].Lessons) != 1 || updated[0].Lessons[0].RunID != "run_test" {
		t.Fatalf("lesson was not recorded: %#v", updated[0].Lessons)
	}
}

func TestToolRecordOutcomeRejectsInvalidKindAndStatus(t *testing.T) {
	dataDir := t.TempDir()
	registry := NewAssetRegistry(dataDir)
	if err := registry.UpsertSkill(SkillAsset{
		Name:     "go-cli-tdd",
		Version:  1,
		Status:   AssetStatusActive,
		Contract: SkillContract{Name: "go-cli-tdd"},
	}); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
	skills, err := registry.ListSkills()
	if err != nil {
		t.Fatalf("list seeded skills: %v", err)
	}
	tool := NewTool(Options{DataDir: dataDir})

	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "invalid kind",
			args: map[string]any{"assetId": skills[0].ID, "kind": "workflow", "status": "success"},
			want: "kind",
		},
		{
			name: "invalid status",
			args: map[string]any{"assetId": skills[0].ID, "kind": AssetKindSkill, "status": "skipped"},
			want: "status",
		},
		{
			name: "nil params",
			args: nil,
			want: "assetId",
		},
		{
			name: "missing assetId",
			args: map[string]any{"kind": AssetKindSkill, "status": "success"},
			want: "assetId",
		},
		{
			name: "missing kind",
			args: map[string]any{"assetId": skills[0].ID, "status": "success"},
			want: "kind",
		},
		{
			name: "missing status",
			args: map[string]any{"assetId": skills[0].ID, "kind": AssetKindSkill},
			want: "status",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tool.RecordOutcome(tc.args)
			if err == nil {
				t.Fatal("expected RecordOutcome to reject invalid args")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func assertWorkflowTasksReferenceAssets(t *testing.T, workflow WorkflowPackage) {
	t.Helper()
	agentIDs := map[string]bool{}
	for _, agent := range workflow.Agents {
		agentIDs[agent.ID] = true
	}
	skillIDs := map[string]bool{}
	for _, skill := range workflow.Skills {
		skillIDs[skill.ID] = true
	}
	for _, task := range workflow.Tasks {
		if !agentIDs[task.AgentID] {
			t.Fatalf("task %q references missing agent %q", task.ID, task.AgentID)
		}
		for _, skillID := range task.SkillIDs {
			if !skillIDs[skillID] {
				t.Fatalf("task %q references missing skill %q", task.ID, skillID)
			}
		}
	}
}

func agentAssetNames(agents []AgentAsset) []string {
	names := make([]string, 0, len(agents))
	for _, agent := range agents {
		names = append(names, agent.Name)
	}
	return names
}

func skillAssetNames(skills []SkillAsset) []string {
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	return names
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

func TestToolRunWritesSpinningWheelEvents(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "시각화 이벤트 테스트", AgentContract{
		Name: "VisualPlannerCat",
		Spec: AgentSpec{
			Role:         "planning cat",
			Capabilities: []string{"planning"},
		},
	}))

	result, err := tool.Run("시각화 이벤트 테스트")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	runID := result["runId"].(string)
	eventsPath := filepath.Join(dataDir, "events", runID+".jsonl")
	file, err := os.Open(eventsPath)
	if err != nil {
		t.Fatalf("expected spinning wheel events file: %v", err)
	}
	defer file.Close()

	types := map[string]bool{}
	agentNames := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event VisualizationEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("event is not JSON: %v", err)
		}
		if event.RunID != runID {
			t.Fatalf("event runId mismatch: %#v", event)
		}
		types[event.Type] = true
		if event.Agent != nil {
			agentNames[event.Agent.Name] = true
			if event.Agent.Avatar.Kind == "" {
				t.Fatalf("expected animal avatar on agent event: %#v", event)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan events: %v", err)
	}

	for _, eventType := range []string{
		EventRunStarted,
		EventAgentSpawned,
		EventStageStarted,
		EventAgentDecision,
		EventStageCompleted,
		EventMetricsUpdated,
		EventRunCompleted,
	} {
		if !types[eventType] {
			t.Fatalf("missing event type %q in %#v", eventType, types)
		}
	}
	if !agentNames["VisualPlannerCat"] {
		t.Fatalf("expected agent event for VisualPlannerCat, got %#v", agentNames)
	}
}

func TestToolRunWritesUpdatedMetricsEvent(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "metrics 이벤트 테스트", AgentContract{
		Name: "MetricsAwareAgent",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility"},
		},
	}))

	result, err := tool.Run("metrics 이벤트 테스트")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	events, err := NewEventStoreForDataDir(dataDir).List(result["runId"].(string))
	if err != nil {
		t.Fatalf("list events failed: %v", err)
	}
	for _, event := range events {
		if event.Type == EventMetricsUpdated && event.Agent != nil && event.Agent.Name == "MetricsAwareAgent" {
			if event.Metrics == nil {
				t.Fatalf("expected metrics payload in event: %#v", event)
			}
			if event.Metrics.TotalRuns != 1 || event.Metrics.SuccessRuns != 1 || event.Metrics.SuccessRate != 1 {
				t.Fatalf("expected updated metrics in event, got %#v", event.Metrics)
			}
			return
		}
	}
	t.Fatalf("expected metrics-updated event for MetricsAwareAgent in %#v", events)
}

func TestToolRunWritesFallbackSpinningWheelAgentsForKeywordStages(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	result, err := tool.Run("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	events, err := NewEventStoreForDataDir(dataDir).List(result["runId"].(string))
	if err != nil {
		t.Fatalf("list events failed: %v", err)
	}
	agentsByStage := map[string]string{}
	avatarsByStage := map[string]string{}
	messages := map[string]bool{}
	for _, event := range events {
		if event.Agent != nil {
			agentsByStage[event.Stage] = event.Agent.Name
			avatarsByStage[event.Stage] = event.Agent.Avatar.Kind
		}
		if event.Message != "" {
			messages[event.Message] = true
		}
	}
	for _, stage := range []string{"planner", "code-writer", "tester", "documenter"} {
		if agentsByStage[stage] == "" {
			t.Fatalf("expected fallback visualization agent for stage %s in %#v", stage, events)
		}
	}
	if !messages["I'm heading to the planning desk."] {
		t.Fatalf("expected speech bubble message for movement, got %#v", messages)
	}
	if avatarsByStage["code-writer"] != "dog" {
		t.Fatalf("expected code writer fallback avatar to be dog, got %#v", avatarsByStage)
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

func TestToolRunUsesAnalyzerPlan(t *testing.T) {
	dataDir := t.TempDir()
	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan("문서만 작성", []StageDef{
		{Stage: "documenter", Keyword: "llm"},
	})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{
		result: AnalysisResult{
			Intent:     IntentResult{Category: IntentDocumenter, Confidence: 0.9},
			Complexity: ComplexityResult{Level: "LOW", Score: 1},
			Plan:       plan,
			Source:     AnalyzerLLM,
		},
	})

	result, err := tool.Run("문서만 작성")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if result["analyzer"] != AnalyzerLLM {
		t.Fatalf("expected llm analyzer, got %#v", result["analyzer"])
	}
	summary := result["summary"].(Summary)
	if summary.TotalStages != 1 {
		t.Fatalf("expected analyzer plan to drive one stage, got %#v", summary)
	}

	runID := result["runId"].(string)
	status, err := tool.GetStatus(runID)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stages := status["stages"].([]StageResult)
	if len(stages) != 1 || stages[0].Stage != "documenter" {
		t.Fatalf("expected documenter-only run, got %#v", stages)
	}
	if status["analyzer"] != AnalyzerLLM {
		t.Fatalf("expected analyzer in status, got %#v", status["analyzer"])
	}
}

func TestToolPropagatesFallbackReason(t *testing.T) {
	dataDir := t.TempDir()
	orchestrator := NewOrchestrator(dataDir)
	plan := orchestrator.ParseRequest("기획 구현 테스트 문서화")
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{
		result: AnalysisResult{
			Intent:         IntentResult{Category: IntentPlanner, Confidence: 0.2},
			Complexity:     ComplexityResult{Level: "LOW", Score: 0},
			Plan:           plan,
			Source:         AnalyzerKeyword,
			FallbackReason: "llm analyzer returned invalid stage",
		},
	})

	analyzed, err := tool.Analyze("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if analyzed["fallbackReason"] != "llm analyzer returned invalid stage" {
		t.Fatalf("expected analyze fallback reason, got %#v", analyzed["fallbackReason"])
	}

	run, err := tool.Run("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if run["fallbackReason"] != "llm analyzer returned invalid stage" {
		t.Fatalf("expected run fallback reason, got %#v", run["fallbackReason"])
	}

	status, err := tool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status["fallbackReason"] != "llm analyzer returned invalid stage" {
		t.Fatalf("expected status fallback reason, got %#v", status["fallbackReason"])
	}
}

func TestToolAnalyzeAndRunExposeDynamicAgents(t *testing.T) {
	dataDir := t.TempDir()
	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan("접근성 좋은 계산기 앱 구현", []StageDef{
		{
			Stage:   "code-writer",
			Keyword: "llm",
			Agent: &AgentContract{
				Name:        "AccessibilityFocusedFrontendAgent",
				Description: "Builds UI code with accessibility checks.",
				Spec: AgentSpec{
					Role:         "frontend accessibility engineer",
					Capabilities: []string{"ui-implementation", "accessibility", "validation"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{
		result: AnalysisResult{
			Intent:     IntentResult{Category: IntentCodeWriter, Confidence: 0.91},
			Complexity: ComplexityResult{Level: "MEDIUM", Score: 4},
			Plan:       plan,
			Source:     AnalyzerLLM,
		},
	})

	analyzed, err := tool.Analyze("접근성 좋은 계산기 앱 구현")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	analyzedPlan := analyzed["plan"].(Plan)
	if analyzedPlan.Stages[0].Agent == nil || analyzedPlan.Stages[0].Agent.Name != "AccessibilityFocusedFrontendAgent" {
		t.Fatalf("expected dynamic agent in analyze plan, got %#v", analyzedPlan.Stages)
	}

	run, err := tool.Run("접근성 좋은 계산기 앱 구현")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	status, err := tool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stages := status["stages"].([]StageResult)
	if stages[0].Stage != "code-writer" || stages[0].Result == nil {
		t.Fatalf("expected code writer execution, got %#v", stages)
	}
	if stages[0].Agent == nil || stages[0].Agent.Name != "AccessibilityFocusedFrontendAgent" {
		t.Fatalf("expected selected dynamic agent in stage result, got %#v", stages[0].Agent)
	}
}

func TestToolRunReusesAndEnhancesSimilarAgentByDefault(t *testing.T) {
	dataDir := t.TempDir()
	firstTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "기존 접근성 UI 에이전트 생성", AgentContract{
		Name:        "ExistingAccessibilityAgent",
		Description: "Builds accessible interfaces.",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility", "semantic-html"},
		},
	}))
	if _, err := firstTool.Run("기존 접근성 UI 에이전트 생성"); err != nil {
		t.Fatalf("initial run failed: %v", err)
	}

	secondTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "접근성 좋은 대시보드 구현", AgentContract{
		Name:        "DashboardAccessibilityAgent",
		Description: "Builds accessible dashboards.",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility", "dashboard-ui"},
		},
	}))
	run, err := secondTool.Run("접근성 좋은 대시보드 구현")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	status, err := secondTool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stage := status["stages"].([]StageResult)[0]
	if stage.Agent == nil || stage.Agent.Name != "ExistingAccessibilityAgent" {
		t.Fatalf("expected similar agent to be reused, got %#v", stage.Agent)
	}
	if stage.AgentDecision == nil || stage.AgentDecision.Action != AgentPolicyReuseEnhance {
		t.Fatalf("expected reuse-enhance decision, got %#v", stage.AgentDecision)
	}
	if stage.AgentDecision.Source != AgentDecisionSourceDefault {
		t.Fatalf("expected default decision source, got %#v", stage.AgentDecision)
	}
	if !containsString(stage.Agent.Spec.Capabilities, "dashboard-ui") {
		t.Fatalf("expected reused agent to gain new capability, got %#v", stage.Agent.Spec.Capabilities)
	}
}

func TestToolRunCanRewriteSimilarAgent(t *testing.T) {
	dataDir := t.TempDir()
	firstTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "기존 문서 에이전트 생성", AgentContract{
		Name:        "LegacyDocumentationAgent",
		Description: "Writes README files.",
		Spec: AgentSpec{
			Role:         "documentation engineer",
			Capabilities: []string{"documentation", "readme"},
		},
	}))
	if _, err := firstTool.Run("기존 문서 에이전트 생성"); err != nil {
		t.Fatalf("initial run failed: %v", err)
	}

	rewriteTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "문서 에이전트를 새 기준으로 재작성", AgentContract{
		Name:        "ModernDocumentationAgent",
		Description: "Writes product guides.",
		Spec: AgentSpec{
			Role:         "documentation engineer",
			Capabilities: []string{"documentation", "product-guide"},
		},
	}))
	run, err := rewriteTool.Run("문서 에이전트를 새 기준으로 재작성", map[string]any{"agentPolicy": AgentPolicyRewrite})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	status, err := rewriteTool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stage := status["stages"].([]StageResult)[0]
	if stage.Agent == nil || stage.Agent.Name != "ModernDocumentationAgent" {
		t.Fatalf("expected rewritten agent candidate to be selected, got %#v", stage.Agent)
	}
	if stage.AgentDecision == nil || stage.AgentDecision.Action != AgentPolicyRewrite {
		t.Fatalf("expected rewrite decision, got %#v", stage.AgentDecision)
	}
	if stage.AgentDecision.ExistingAgentName != "LegacyDocumentationAgent" {
		t.Fatalf("expected decision to identify rewritten agent, got %#v", stage.AgentDecision)
	}
}

func TestToolRunCanSeparateSimilarAgent(t *testing.T) {
	dataDir := t.TempDir()
	firstTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "기존 분석 에이전트 생성", AgentContract{
		Name:        "GeneralAnalysisAgent",
		Description: "Analyzes product metrics.",
		Spec: AgentSpec{
			Role:         "data analyst",
			Capabilities: []string{"data-analysis", "reporting"},
		},
	}))
	if _, err := firstTool.Run("기존 분석 에이전트 생성"); err != nil {
		t.Fatalf("initial run failed: %v", err)
	}

	separateTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "별도 실험 분석 에이전트 생성", AgentContract{
		Name:        "ExperimentAnalysisAgent",
		Description: "Analyzes experiments separately.",
		Spec: AgentSpec{
			Role:         "data analyst",
			Capabilities: []string{"data-analysis", "experimentation"},
		},
	}))
	run, err := separateTool.Run("별도 실험 분석 에이전트 생성", map[string]any{"agentPolicy": AgentPolicySeparate})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	status, err := separateTool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stage := status["stages"].([]StageResult)[0]
	if stage.Agent == nil || stage.Agent.Name != "ExperimentAnalysisAgent" {
		t.Fatalf("expected candidate to remain separate, got %#v", stage.Agent)
	}
	if stage.AgentDecision == nil || stage.AgentDecision.Action != AgentPolicySeparate {
		t.Fatalf("expected separate decision, got %#v", stage.AgentDecision)
	}
	if stage.AgentDecision.ExistingAgentName != "GeneralAnalysisAgent" {
		t.Fatalf("expected decision to identify similar existing agent, got %#v", stage.AgentDecision)
	}
}

func TestToolRunHonorsAnalyzerAgentLifecycleDecision(t *testing.T) {
	dataDir := t.TempDir()
	firstTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "기존 접근성 에이전트 생성", AgentContract{
		Name:        "LegacyAccessibilityAgent",
		Description: "Builds basic accessible interfaces.",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility", "semantic-html"},
		},
	}))
	if _, err := firstTool.Run("기존 접근성 에이전트 생성"); err != nil {
		t.Fatalf("initial run failed: %v", err)
	}

	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan("접근성 에이전트 재작성", []StageDef{{
		Stage:   "code-writer",
		Keyword: "llm",
		Agent: &AgentContract{
			Name:        "ModernAccessibilityAgent",
			Description: "Builds modern accessible interfaces.",
			Spec: AgentSpec{
				Role:         "frontend accessibility engineer",
				Capabilities: []string{"accessibility", "design-system"},
			},
		},
		AgentDecision: &AgentLifecycleDecision{
			Action:            AgentPolicyRewrite,
			ExistingAgentName: "LegacyAccessibilityAgent",
			Reason:            "LLM selected rewrite",
		},
	}})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{result: AnalysisResult{
		Intent:     IntentResult{Category: IntentCodeWriter, Confidence: 0.91},
		Complexity: ComplexityResult{Level: "MEDIUM", Score: 4},
		Plan:       plan,
		Source:     AnalyzerLLM,
	}})

	run, err := tool.Run("접근성 에이전트 재작성")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	status, err := tool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stage := status["stages"].([]StageResult)[0]
	if stage.Agent == nil || stage.Agent.Name != "ModernAccessibilityAgent" {
		t.Fatalf("expected analyzer rewrite candidate to be selected, got %#v", stage.Agent)
	}
	if stage.AgentDecision == nil || stage.AgentDecision.Action != AgentPolicyRewrite {
		t.Fatalf("expected analyzer rewrite decision, got %#v", stage.AgentDecision)
	}
	if stage.AgentDecision.Source != AgentDecisionSourceStageDecision {
		t.Fatalf("expected analyzer decision source, got %#v", stage.AgentDecision)
	}
	if stage.AgentDecision.Reason != "LLM selected rewrite" {
		t.Fatalf("expected analyzer decision reason to be preserved, got %#v", stage.AgentDecision)
	}
}

func TestToolListAgentsReturnsPersistedContracts(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "접근성 UI 에이전트 생성", AgentContract{
		Name: "InspectableAccessibilityAgent",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility"},
		},
	}))
	if _, err := tool.Run("접근성 UI 에이전트 생성"); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	listed, err := tool.ListAgents()
	if err != nil {
		t.Fatalf("list agents failed: %v", err)
	}
	if listed["action"] != "list_agents" {
		t.Fatalf("expected list_agents action, got %#v", listed)
	}
	agents := listed["agents"].([]AgentContract)
	if len(agents) != 1 || agents[0].Name != "InspectableAccessibilityAgent" {
		t.Fatalf("expected persisted agent contract, got %#v", agents)
	}
}

func TestToolRecordsSuccessfulAgentPerformance(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "성공하는 접근성 UI 에이전트", AgentContract{
		Name: "SuccessfulAccessibilityAgent",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility"},
		},
	}))
	if _, err := tool.Run("성공하는 접근성 UI 에이전트"); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	listed, err := tool.ListAgents()
	if err != nil {
		t.Fatalf("list agents failed: %v", err)
	}
	agents := listed["agents"].([]AgentContract)
	if len(agents) != 1 {
		t.Fatalf("expected one agent, got %#v", agents)
	}
	metrics := agents[0].Metrics
	if metrics.TotalRuns != 1 || metrics.SuccessRuns != 1 || metrics.FailureRuns != 0 {
		t.Fatalf("unexpected success metrics: %#v", metrics)
	}
	if metrics.SuccessRate != 1 || metrics.LastStatus != "success" || metrics.LastRunAt.IsZero() {
		t.Fatalf("expected success status and timestamp, got %#v", metrics)
	}
}

func TestToolRecordsFailedAgentPerformance(t *testing.T) {
	dataDir := t.TempDir()
	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan("테스트만 실행", []StageDef{{
		Stage:   "tester",
		Keyword: "llm",
		Agent: &AgentContract{
			Name: "FailureAwareTesterAgent",
			Spec: AgentSpec{
				Role:         "test engineer",
				Capabilities: []string{"validation"},
			},
		},
	}})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{result: AnalysisResult{
		Intent:     IntentResult{Category: IntentGeneral, Confidence: 0.7},
		Complexity: ComplexityResult{Level: "LOW", Score: 1},
		Plan:       plan,
		Source:     AnalyzerLLM,
	}})

	run, err := tool.Run("테스트만 실행")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if run["status"] != "error" {
		t.Fatalf("expected failed run status, got %#v", run)
	}

	listed, err := tool.ListAgents()
	if err != nil {
		t.Fatalf("list agents failed: %v", err)
	}
	agents := listed["agents"].([]AgentContract)
	if len(agents) != 1 {
		t.Fatalf("expected one agent, got %#v", agents)
	}
	metrics := agents[0].Metrics
	if metrics.TotalRuns != 1 || metrics.SuccessRuns != 0 || metrics.FailureRuns != 1 {
		t.Fatalf("unexpected failure metrics: %#v", metrics)
	}
	if metrics.SuccessRate != 0 || metrics.LastStatus != "error" || metrics.LastError == "" {
		t.Fatalf("expected failure status and error, got %#v", metrics)
	}
}

func TestToolListAgentsIncludesLifecycleRecommendations(t *testing.T) {
	dataDir := t.TempDir()
	registry := NewAgentRegistry(filepath.Join(dataDir, "agents.json"))
	reliable := AgentContract{
		Name: "ReliableAccessibilityAgent",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility"},
		},
	}
	unstable := AgentContract{
		Name: "UnstableTesterAgent",
		Spec: AgentSpec{
			Role:         "test engineer",
			Capabilities: []string{"validation"},
		},
	}
	if err := registry.RecordStageResults([]StageResult{
		{Stage: "code-writer", Status: "success", Agent: &reliable},
		{Stage: "code-writer", Status: "success", Agent: &reliable},
		{Stage: "code-writer", Status: "success", Agent: &reliable},
		{Stage: "tester", Status: "error", Error: "validation failed", Agent: &unstable},
		{Stage: "tester", Status: "error", Error: "validation failed", Agent: &unstable},
	}); err != nil {
		t.Fatalf("record stage results failed: %v", err)
	}

	successTool := NewTool(Options{DataDir: dataDir})
	listed, err := successTool.ListAgents()
	if err != nil {
		t.Fatalf("list agents failed: %v", err)
	}
	agents := listed["agents"].([]AgentContract)
	reliableAgent := findAgentForTest(agents, "ReliableAccessibilityAgent")
	if reliableAgent == nil {
		t.Fatalf("expected reliable agent in %#v", agents)
	}
	if reliableAgent.LifecycleRecommendation.Action != AgentPolicyReuseEnhance {
		t.Fatalf("expected reliable agent to recommend reuse, got %#v", reliableAgent.LifecycleRecommendation)
	}
	unstableAgent := findAgentForTest(agents, "UnstableTesterAgent")
	if unstableAgent == nil {
		t.Fatalf("expected unstable agent in %#v", agents)
	}
	if unstableAgent.LifecycleRecommendation.Action != AgentPolicyRewrite {
		t.Fatalf("expected unstable agent to recommend rewrite, got %#v", unstableAgent.LifecycleRecommendation)
	}
	if unstableAgent.LifecycleRecommendation.Reason == "" {
		t.Fatalf("expected recommendation reason, got %#v", unstableAgent.LifecycleRecommendation)
	}
}

func TestToolRunAppliesLifecycleRecommendationWhenPolicyIsImplicit(t *testing.T) {
	dataDir := t.TempDir()
	registry := NewAgentRegistry(filepath.Join(dataDir, "agents.json"))
	existing := AgentContract{
		Name:        "UnstableAccessibilityAgent",
		Description: "Builds accessible interfaces.",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility", "ui-implementation"},
		},
	}
	if err := registry.RecordStageResults([]StageResult{
		{Stage: "code-writer", Status: "error", Error: "compile failed", Agent: &existing},
		{Stage: "code-writer", Status: "error", Error: "compile failed", Agent: &existing},
	}); err != nil {
		t.Fatalf("record stage results failed: %v", err)
	}

	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "접근성 UI 에이전트 재구성", AgentContract{
		Name:        "ModernAccessibilityAgent",
		Description: "Builds accessible interfaces.",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility", "ui-implementation", "design-system"},
		},
	}))
	run, err := tool.Run("접근성 UI 에이전트 재구성")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	status, err := tool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stage := status["stages"].([]StageResult)[0]
	if stage.AgentDecision == nil || stage.AgentDecision.Action != AgentPolicyRewrite {
		t.Fatalf("expected rewrite recommendation to be applied, got %#v", stage.AgentDecision)
	}
	if stage.AgentDecision.Source != AgentDecisionSourceRegistryRecommendation {
		t.Fatalf("expected registry recommendation source, got %#v", stage.AgentDecision)
	}
	if !strings.Contains(stage.AgentDecision.Reason, "repeated failures") {
		t.Fatalf("expected recommendation reason to be preserved, got %#v", stage.AgentDecision)
	}
	if stage.Agent == nil || stage.Agent.Name != "ModernAccessibilityAgent" {
		t.Fatalf("expected rewritten candidate to be selected, got %#v", stage.Agent)
	}
}

func TestToolRunOptionsOverrideLifecycleRecommendation(t *testing.T) {
	dataDir := t.TempDir()
	registry := NewAgentRegistry(filepath.Join(dataDir, "agents.json"))
	existing := AgentContract{
		Name:        "UnstableAnalysisAgent",
		Description: "Analyzes product metrics.",
		Spec: AgentSpec{
			Role:         "data analyst",
			Capabilities: []string{"data-analysis"},
		},
	}
	if err := registry.RecordStageResults([]StageResult{
		{Stage: "code-writer", Status: "error", Error: "analysis failed", Agent: &existing},
		{Stage: "code-writer", Status: "error", Error: "analysis failed", Agent: &existing},
	}); err != nil {
		t.Fatalf("record stage results failed: %v", err)
	}

	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "별도 분석 에이전트 생성", AgentContract{
		Name:        "ExperimentAnalysisAgent",
		Description: "Analyzes product metrics.",
		Spec: AgentSpec{
			Role:         "data analyst",
			Capabilities: []string{"data-analysis", "experimentation"},
		},
	}))
	run, err := tool.Run("별도 분석 에이전트 생성", map[string]any{"agentPolicy": AgentPolicySeparate})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	status, err := tool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stage := status["stages"].([]StageResult)[0]
	if stage.AgentDecision == nil || stage.AgentDecision.Action != AgentPolicySeparate {
		t.Fatalf("expected explicit separate policy to override recommendation, got %#v", stage.AgentDecision)
	}
	if stage.AgentDecision.Source != AgentDecisionSourceRunOption {
		t.Fatalf("expected run option decision source, got %#v", stage.AgentDecision)
	}
	if stage.Agent == nil || stage.Agent.Name != "ExperimentAnalysisAgent" {
		t.Fatalf("expected separate candidate to be selected, got %#v", stage.Agent)
	}
}

func TestToolRefreshesLLMAgentContextBetweenRequests(t *testing.T) {
	dataDir := t.TempDir()
	userMessages := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		userMessages = append(userMessages, payload.Messages[len(payload.Messages)-1].Content)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"intent":"code-writer","confidence":0.8,"keywords":["접근성"],"complexity":{"level":"LOW","score":2},"stages":[{"stage":"code-writer","keyword":"llm","agent":{"name":"SavedAccessibilityAgent","spec":{"role":"frontend accessibility engineer","capabilities":["accessibility"]}}}]}`,
				},
			}},
		})
	}))
	defer server.Close()

	tool := NewTool(Options{
		DataDir:  dataDir,
		Analyzer: AnalyzerLLM,
		LLM: LLMAnalyzerConfig{
			Endpoint: server.URL,
			APIKey:   "test-key",
			Model:    "test-model",
		},
	})
	if _, err := tool.Run("접근성 에이전트 생성"); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if _, err := tool.Analyze("저장된 에이전트 참고해서 분석"); err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if len(userMessages) != 2 {
		t.Fatalf("expected two LLM calls, got %#v", userMessages)
	}
	if !strings.Contains(userMessages[1], "SavedAccessibilityAgent") {
		t.Fatalf("expected second LLM request to include refreshed agent context, got %s", userMessages[1])
	}
	if !strings.Contains(userMessages[1], "successRate") {
		t.Fatalf("expected second LLM request to include agent performance metrics, got %s", userMessages[1])
	}
	if !strings.Contains(userMessages[1], "lifecycleRecommendation") {
		t.Fatalf("expected second LLM request to include lifecycle recommendation, got %s", userMessages[1])
	}
}

func analyzerForAgentPlan(t *testing.T, dataDir string, request string, agent AgentContract) Analyzer {
	t.Helper()
	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan(request, []StageDef{{
		Stage:   "code-writer",
		Keyword: "llm",
		Agent:   &agent,
	}})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	return staticAnalyzer{
		result: AnalysisResult{
			Intent:     IntentResult{Category: IntentCodeWriter, Confidence: 0.91},
			Complexity: ComplexityResult{Level: "MEDIUM", Score: 4},
			Plan:       plan,
			Source:     AnalyzerLLM,
		},
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func findAgentForTest(agents []AgentContract, name string) *AgentContract {
	for index := range agents {
		if agents[index].Name == name {
			return &agents[index]
		}
	}
	return nil
}
