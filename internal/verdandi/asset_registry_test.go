package verdandi

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAssetStatusAllowsEvolutionWithoutDeletion(t *testing.T) {
	statuses := assetStatusOptions()
	want := []string{
		AssetStatusActive,
		AssetStatusNeedsReview,
		AssetStatusSuperseded,
		AssetStatusDeprecated,
		AssetStatusArchived,
	}
	if len(statuses) != len(want) {
		t.Fatalf("expected %d statuses, got %d: %#v", len(want), len(statuses), statuses)
	}
	for index, status := range want {
		if statuses[index] != status {
			t.Fatalf("status %d = %q, want %q", index, statuses[index], status)
		}
	}
}

func TestAssetMetricsFromOutcomeComputesSuccessRate(t *testing.T) {
	metrics := AssetMetrics{}
	ended := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)

	metrics = updateAssetMetrics(metrics, AssetOutcome{Status: "success", CompletedAt: ended})
	metrics = updateAssetMetrics(metrics, AssetOutcome{Status: "error", Error: "tests failed", CompletedAt: ended.Add(time.Minute)})

	if metrics.TotalRuns != 2 {
		t.Fatalf("TotalRuns = %d, want 2", metrics.TotalRuns)
	}
	if metrics.SuccessRuns != 1 {
		t.Fatalf("SuccessRuns = %d, want 1", metrics.SuccessRuns)
	}
	if metrics.FailureRuns != 1 {
		t.Fatalf("FailureRuns = %d, want 1", metrics.FailureRuns)
	}
	if metrics.SuccessRate != 0.5 {
		t.Fatalf("SuccessRate = %.2f, want 0.50", metrics.SuccessRate)
	}
	if metrics.LastError != "tests failed" {
		t.Fatalf("LastError = %q, want tests failed", metrics.LastError)
	}
	if metrics.LastUsedAt == nil || !metrics.LastUsedAt.Equal(ended.Add(time.Minute)) {
		t.Fatalf("LastUsedAt = %v, want %v", metrics.LastUsedAt, ended.Add(time.Minute))
	}
}

func TestAgentAssetJSONUsesSingleMetricsSource(t *testing.T) {
	asset := AgentAsset{
		ID:      "agent-1",
		Name:    "GoCliImplementer",
		Role:    "code-writer",
		Version: 1,
		Status:  AssetStatusActive,
		Contract: AgentContract{
			Name: "GoCliImplementer",
			Metrics: AgentMetrics{
				TotalRuns:   99,
				SuccessRuns: 1,
				FailureRuns: 98,
			},
			LifecycleRecommendation: AgentLifecycleRecommendation{
				Action: AgentPolicyRewrite,
				Reason: "nested recommendation should not persist",
			},
		},
		Metrics: AssetMetrics{TotalRuns: 2, SuccessRuns: 2, SuccessRate: 1},
	}

	encoded, err := json.Marshal(asset)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(encoded, &doc); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if _, ok := doc["metrics"]; !ok {
		t.Fatalf("expected top-level metrics in %s", encoded)
	}
	contract, ok := doc["contract"].(map[string]any)
	if !ok {
		t.Fatalf("expected contract object in %s", encoded)
	}
	if _, ok := contract["metrics"]; ok {
		t.Fatalf("contract.metrics should be omitted from %s", encoded)
	}
	if _, ok := contract["lifecycleRecommendation"]; ok {
		t.Fatalf("contract.lifecycleRecommendation should be omitted from %s", encoded)
	}
}

func TestEmptyAssetMetricsOmitsLastUsedAt(t *testing.T) {
	encoded, err := json.Marshal(AssetMetrics{})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(encoded, &doc); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if _, ok := doc["lastUsedAt"]; ok {
		t.Fatalf("lastUsedAt should be omitted from empty metrics JSON: %s", encoded)
	}
}
