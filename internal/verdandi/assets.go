package verdandi

import (
	"encoding/json"
	"time"
)

const (
	AssetKindAgent = "agent"
	AssetKindSkill = "skill"
)

const (
	AssetStatusActive      = "active"
	AssetStatusNeedsReview = "needs-review"
	AssetStatusSuperseded  = "superseded"
	AssetStatusDeprecated  = "deprecated"
	AssetStatusArchived    = "archived"
)

type AssetMetrics struct {
	TotalRuns    int        `json:"totalRuns"`
	SuccessRuns  int        `json:"successRuns"`
	FailureRuns  int        `json:"failureRuns"`
	SuccessRate  float64    `json:"successRate"`
	LastStatus   string     `json:"lastStatus,omitempty"`
	LastError    string     `json:"lastError,omitempty"`
	LastUsedAt   *time.Time `json:"lastUsedAt,omitempty"`
	CommonErrors []string   `json:"commonErrors,omitempty"`
}

type AssetLineage struct {
	ParentID      string    `json:"parentId,omitempty"`
	SupersedesIDs []string  `json:"supersedesIds,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Lesson struct {
	RunID     string    `json:"runId,omitempty"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"createdAt"`
}

type SkillContract struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	WhenToUse    string            `json:"whenToUse"`
	Instructions string            `json:"instructions"`
	Inputs       []string          `json:"inputs"`
	Outputs      []string          `json:"outputs"`
	Constraints  []string          `json:"constraints"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type AgentAsset struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Role     string        `json:"role"`
	Version  int           `json:"version"`
	Status   string        `json:"status"`
	Contract AgentContract `json:"contract"`
	Skills   []string      `json:"skills,omitempty"`
	Metrics  AssetMetrics  `json:"metrics"`
	Lineage  AssetLineage  `json:"lineage"`
	Lessons  []Lesson      `json:"lessons,omitempty"`
}

func (asset AgentAsset) MarshalJSON() ([]byte, error) {
	type agentContractJSON struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Command     string            `json:"command"`
		Spec        AgentSpec         `json:"spec"`
		Metadata    map[string]any    `json:"metadata"`
		Inputs      map[string]string `json:"inputs"`
	}
	type agentAssetJSON struct {
		ID       string            `json:"id"`
		Name     string            `json:"name"`
		Role     string            `json:"role"`
		Version  int               `json:"version"`
		Status   string            `json:"status"`
		Contract agentContractJSON `json:"contract"`
		Skills   []string          `json:"skills,omitempty"`
		Metrics  AssetMetrics      `json:"metrics"`
		Lineage  AssetLineage      `json:"lineage"`
		Lessons  []Lesson          `json:"lessons,omitempty"`
	}
	return json.Marshal(agentAssetJSON{
		ID:      asset.ID,
		Name:    asset.Name,
		Role:    asset.Role,
		Version: asset.Version,
		Status:  asset.Status,
		Contract: agentContractJSON{
			Name:        asset.Contract.Name,
			Description: asset.Contract.Description,
			Command:     asset.Contract.Command,
			Spec:        asset.Contract.Spec,
			Metadata:    asset.Contract.Metadata,
			Inputs:      asset.Contract.Inputs,
		},
		Skills:  asset.Skills,
		Metrics: asset.Metrics,
		Lineage: asset.Lineage,
		Lessons: asset.Lessons,
	})
}

type SkillAsset struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Version      int           `json:"version"`
	Status       string        `json:"status"`
	Contract     SkillContract `json:"contract"`
	UsedByAgents []string      `json:"usedByAgents,omitempty"`
	Metrics      AssetMetrics  `json:"metrics"`
	Lineage      AssetLineage  `json:"lineage"`
	Lessons      []Lesson      `json:"lessons,omitempty"`
}

type AssetOutcome struct {
	RunID       string    `json:"runId,omitempty"`
	AssetID     string    `json:"assetId,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
	Lesson      string    `json:"lesson,omitempty"`
	CompletedAt time.Time `json:"completedAt"`
}

type AssetLifecycleRecommendation struct {
	Action string  `json:"action"`
	Reason string  `json:"reason"`
	Score  float64 `json:"score,omitempty"`
}

func assetStatusOptions() []string {
	return []string{
		AssetStatusActive,
		AssetStatusNeedsReview,
		AssetStatusSuperseded,
		AssetStatusDeprecated,
		AssetStatusArchived,
	}
}

func updateAssetMetrics(metrics AssetMetrics, outcome AssetOutcome) AssetMetrics {
	metrics.TotalRuns++
	metrics.LastStatus = outcome.Status
	if !outcome.CompletedAt.IsZero() {
		completedAt := outcome.CompletedAt
		metrics.LastUsedAt = &completedAt
	}
	if outcome.Status == "success" {
		metrics.SuccessRuns++
		metrics.LastError = ""
	} else {
		metrics.FailureRuns++
		metrics.LastError = outcome.Error
		if outcome.Error != "" {
			metrics.CommonErrors = mergeStrings(metrics.CommonErrors, []string{outcome.Error})
		}
	}
	if metrics.TotalRuns > 0 {
		metrics.SuccessRate = float64(metrics.SuccessRuns) / float64(metrics.TotalRuns)
	}
	return metrics
}
