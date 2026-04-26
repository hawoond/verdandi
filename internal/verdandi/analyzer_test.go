package verdandi

import "testing"

func TestKeywordAnalyzerReturnsAnalysisAndPlan(t *testing.T) {
	analyzer := NewKeywordAnalyzer(NewOrchestrator(t.TempDir()))

	result, err := analyzer.Analyze("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if result.Intent.Category != IntentPlanner {
		t.Fatalf("expected planner intent, got %#v", result.Intent)
	}
	if result.Plan.StageCount != 4 {
		t.Fatalf("expected 4 stages, got %#v", result.Plan)
	}
	got := []string{}
	for _, stage := range result.Plan.Stages {
		got = append(got, stage.Stage)
	}
	want := []string{"planner", "code-writer", "tester", "documenter"}
	if !equalStrings(got, want) {
		t.Fatalf("stages mismatch: got %#v want %#v", got, want)
	}
}

func TestNewAnalyzerDefaultsToKeyword(t *testing.T) {
	analyzer := NewAnalyzer(AnalyzerConfig{
		Mode:         "",
		Orchestrator: NewOrchestrator(t.TempDir()),
	})

	if _, ok := analyzer.(KeywordAnalyzer); !ok {
		t.Fatalf("expected keyword analyzer default, got %T", analyzer)
	}
}

func TestNormalizePlanRejectsUnknownStages(t *testing.T) {
	orchestrator := NewOrchestrator(t.TempDir())
	_, err := orchestrator.NormalizePlan("do something", []StageDef{
		{Stage: "shell", Keyword: "rm", Order: 1},
	})
	if err == nil {
		t.Fatal("expected unknown stage error")
	}
}

func TestNormalizePlanOrdersStagesAndAddsTester(t *testing.T) {
	orchestrator := NewOrchestrator(t.TempDir())
	plan, err := orchestrator.NormalizePlan("구현 문서화", []StageDef{
		{Stage: "documenter", Keyword: "llm", Order: 1},
		{Stage: "code-writer", Keyword: "llm", Order: 2},
	})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	got := []string{}
	for _, stage := range plan.Stages {
		got = append(got, stage.Stage)
	}
	want := []string{"code-writer", "tester", "documenter"}
	if !equalStrings(got, want) {
		t.Fatalf("stages mismatch: got %#v want %#v", got, want)
	}
}
