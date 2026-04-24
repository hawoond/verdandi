package verdandi

import "testing"

func TestClassifierDetectsOrchestratorIntent(t *testing.T) {
	result := NewClassifier().Analyze("작업을 분석하고 필요한 에이전트를 동적으로 생성해서 연계 실행")

	if result.Intent.Category != IntentOrchestrator {
		t.Fatalf("expected orchestrator intent, got %q", result.Intent.Category)
	}
	if result.Intent.Confidence <= 0 {
		t.Fatalf("expected positive confidence, got %f", result.Intent.Confidence)
	}
}

func TestClassifierFallsBackToGeneral(t *testing.T) {
	result := NewClassifier().Analyze("zzzz qqqq xxxx")

	if result.Intent.Category != IntentGeneral {
		t.Fatalf("expected general intent, got %q", result.Intent.Category)
	}
	if result.Intent.Confidence != 0 {
		t.Fatalf("expected zero confidence, got %f", result.Intent.Confidence)
	}
}
