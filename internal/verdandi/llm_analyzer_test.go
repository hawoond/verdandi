package verdandi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLLMAnalyzerParsesPlanFromCompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing bearer token: %s", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"intent":"orchestrator","confidence":0.91,"keywords":["계산기","구현"],"complexity":{"level":"MEDIUM","score":4},"stages":[{"stage":"planner","keyword":"llm"},{"stage":"code-writer","keyword":"llm"},{"stage":"documenter","keyword":"llm"}]}`,
				},
			}},
		})
	}))
	defer server.Close()

	analyzer := NewLLMAnalyzer(LLMAnalyzerConfig{
		Endpoint: server.URL,
		Model:    "test-model",
		APIKey:   "test-key",
	}, NewKeywordAnalyzer(NewOrchestrator(t.TempDir())))

	result, err := analyzer.Analyze("계산기 앱 만들어줘")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if result.Source != AnalyzerLLM {
		t.Fatalf("expected llm source, got %s", result.Source)
	}
	if result.Intent.Category != IntentOrchestrator {
		t.Fatalf("unexpected intent: %#v", result.Intent)
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

func TestLLMAnalyzerFallsBackWhenOutputIsInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"intent":"orchestrator","confidence":0.91,"stages":[{"stage":"shell","keyword":"bad"}]}`,
				},
			}},
		})
	}))
	defer server.Close()

	analyzer := NewLLMAnalyzer(LLMAnalyzerConfig{
		Endpoint: server.URL,
		Model:    "test-model",
		APIKey:   "test-key",
	}, NewKeywordAnalyzer(NewOrchestrator(t.TempDir())))

	result, err := analyzer.Analyze("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("fallback analyze failed: %v", err)
	}
	if result.Source != AnalyzerKeyword {
		t.Fatalf("expected keyword fallback source, got %s", result.Source)
	}
	if result.Plan.StageCount != 4 {
		t.Fatalf("expected keyword fallback plan, got %#v", result.Plan)
	}
}
