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

func TestAssetRegistryPersistsAgentsAndSkills(t *testing.T) {
	registry := NewAssetRegistry(t.TempDir())
	now := time.Date(2026, 5, 9, 11, 0, 0, 0, time.UTC)

	agent := AgentAsset{
		Name:    "GoCliImplementer",
		Role:    "code-writer",
		Version: 1,
		Status:  AssetStatusActive,
		Contract: AgentContract{
			Name:        "GoCliImplementer",
			Description: "Implements Go CLI features for a coding agent handoff.",
			Command:     "codex",
			Spec: AgentSpec{
				Role:         "code-writer",
				Capabilities: []string{"go", "cli", "tdd"},
			},
			Inputs: map[string]string{"request": "계산기 CLI"},
		},
		Lineage: AssetLineage{CreatedAt: now, UpdatedAt: now},
	}
	skill := SkillAsset{
		Name:    "go-cli-tdd",
		Version: 1,
		Status:  AssetStatusActive,
		Contract: SkillContract{
			Name:         "go-cli-tdd",
			Description:  "Guide Go CLI implementation with tests first.",
			WhenToUse:    "Use for Go command-line tools.",
			Instructions: "Write failing tests before implementation.",
			Inputs:       []string{"request", "acceptanceCriteria"},
			Outputs:      []string{"patch", "testResults"},
			Constraints:  []string{"Do not generate code inside Verdandi."},
		},
		Lineage: AssetLineage{CreatedAt: now, UpdatedAt: now},
	}

	if err := registry.UpsertAgent(agent); err != nil {
		t.Fatalf("UpsertAgent returned error: %v", err)
	}
	if err := registry.UpsertSkill(skill); err != nil {
		t.Fatalf("UpsertSkill returned error: %v", err)
	}

	loadedAgents, err := registry.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents returned error: %v", err)
	}
	loadedSkills, err := registry.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills returned error: %v", err)
	}
	if len(loadedAgents) != 1 || loadedAgents[0].ID == "" {
		t.Fatalf("expected one persisted agent with ID, got %#v", loadedAgents)
	}
	if len(loadedSkills) != 1 || loadedSkills[0].ID == "" {
		t.Fatalf("expected one persisted skill with ID, got %#v", loadedSkills)
	}
}

func TestAssetRegistryCreatesNewVersionInsteadOfDeleting(t *testing.T) {
	registry := NewAssetRegistry(t.TempDir())
	first := AgentAsset{
		Name:     "GoCliImplementer",
		Role:     "code-writer",
		Version:  1,
		Status:   AssetStatusActive,
		Contract: AgentContract{Name: "GoCliImplementer", Spec: AgentSpec{Role: "code-writer", Capabilities: []string{"go", "cli"}}},
	}
	second := first
	second.Contract.Spec.Capabilities = []string{"go", "cli", "tdd"}

	savedFirst, err := registry.SaveAgentVersion(first, "")
	if err != nil {
		t.Fatalf("SaveAgentVersion first returned error: %v", err)
	}
	savedSecond, err := registry.SaveAgentVersion(second, savedFirst.ID)
	if err != nil {
		t.Fatalf("SaveAgentVersion second returned error: %v", err)
	}

	agents, err := registry.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents returned error: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected two versions to remain, got %d: %#v", len(agents), agents)
	}
	if savedSecond.Version != 2 {
		t.Fatalf("second Version = %d, want 2", savedSecond.Version)
	}
	if agents[0].Status == "" || agents[1].Status == "" {
		t.Fatalf("all agents should keep explicit lifecycle status: %#v", agents)
	}
	agentsByID := map[string]AgentAsset{}
	for _, agent := range agents {
		agentsByID[agent.ID] = agent
	}
	if agentsByID[savedFirst.ID].Status != AssetStatusSuperseded {
		t.Fatalf("parent Status = %q, want %q", agentsByID[savedFirst.ID].Status, AssetStatusSuperseded)
	}
	if agentsByID[savedSecond.ID].Status != AssetStatusActive {
		t.Fatalf("new version Status = %q, want %q", agentsByID[savedSecond.ID].Status, AssetStatusActive)
	}
	if agentsByID[savedSecond.ID].Lineage.ParentID != savedFirst.ID {
		t.Fatalf("new version ParentID = %q, want %q", agentsByID[savedSecond.ID].Lineage.ParentID, savedFirst.ID)
	}
}

func TestAssetRegistrySaveAgentVersionRecomputesIDFromSavedTemplate(t *testing.T) {
	registry := NewAssetRegistry(t.TempDir())
	first := AgentAsset{
		Name:     "GoCliImplementer",
		Role:     "code-writer",
		Contract: AgentContract{Name: "GoCliImplementer", Spec: AgentSpec{Role: "code-writer"}},
	}

	savedFirst, err := registry.SaveAgentVersion(first, "")
	if err != nil {
		t.Fatalf("SaveAgentVersion first returned error: %v", err)
	}
	second := savedFirst
	second.Contract.Spec.Capabilities = []string{"go", "cli", "tdd"}
	savedSecond, err := registry.SaveAgentVersion(second, savedFirst.ID)
	if err != nil {
		t.Fatalf("SaveAgentVersion second returned error: %v", err)
	}

	if savedSecond.ID == savedFirst.ID {
		t.Fatalf("second ID should not reuse first ID: %q", savedSecond.ID)
	}
	if savedSecond.ID != assetID(AssetKindAgent, "GoCliImplementer", 2) {
		t.Fatalf("second ID = %q, want %q", savedSecond.ID, assetID(AssetKindAgent, "GoCliImplementer", 2))
	}
}

func TestAssetRegistrySaveAgentVersionUsesContractNameForVersioning(t *testing.T) {
	registry := NewAssetRegistry(t.TempDir())
	first := AgentAsset{
		Contract: AgentContract{Name: "GoCliImplementer", Spec: AgentSpec{Role: "code-writer", Capabilities: []string{"go", "cli"}}},
	}
	second := AgentAsset{
		Contract: AgentContract{Name: "GoCliImplementer", Spec: AgentSpec{Role: "code-writer", Capabilities: []string{"go", "cli", "tdd"}}},
	}

	savedFirst, err := registry.SaveAgentVersion(first, "")
	if err != nil {
		t.Fatalf("SaveAgentVersion first returned error: %v", err)
	}
	savedSecond, err := registry.SaveAgentVersion(second, savedFirst.ID)
	if err != nil {
		t.Fatalf("SaveAgentVersion second returned error: %v", err)
	}

	if savedSecond.Version != 2 {
		t.Fatalf("second Version = %d, want 2", savedSecond.Version)
	}
	agents, err := registry.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents returned error: %v", err)
	}
	agentsByID := map[string]AgentAsset{}
	for _, agent := range agents {
		agentsByID[agent.ID] = agent
	}
	if agentsByID[savedFirst.ID].Status != AssetStatusSuperseded {
		t.Fatalf("parent Status = %q, want %q", agentsByID[savedFirst.ID].Status, AssetStatusSuperseded)
	}
}
