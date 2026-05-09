package verdandi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type AssetRegistry struct {
	path string
}

type assetRegistryFile struct {
	Agents []AgentAsset `json:"agents"`
	Skills []SkillAsset `json:"skills"`
}

func NewAssetRegistry(dataDir string) AssetRegistry {
	return AssetRegistry{path: filepath.Join(dataDir, "registry", "assets.json")}
}

func (r AssetRegistry) ListAgents() ([]AgentAsset, error) {
	store, err := r.load()
	if err != nil {
		return nil, err
	}
	agents := append([]AgentAsset{}, store.Agents...)
	sortAgentAssets(agents)
	return agents, nil
}

func (r AssetRegistry) ListSkills() ([]SkillAsset, error) {
	store, err := r.load()
	if err != nil {
		return nil, err
	}
	skills := append([]SkillAsset{}, store.Skills...)
	sortSkillAssets(skills)
	return skills, nil
}

func (r AssetRegistry) UpsertAgent(agent AgentAsset) error {
	lock := lockForPath(r.path)
	lock.Lock()
	defer lock.Unlock()

	store, err := r.load()
	if err != nil {
		return err
	}
	agent = normalizeAgentAsset(agent, "")
	for index, existing := range store.Agents {
		if existing.ID == agent.ID || (strings.EqualFold(existing.Name, agent.Name) && existing.Version == agent.Version) {
			store.Agents[index] = agent
			return r.write(store)
		}
	}
	store.Agents = append(store.Agents, agent)
	return r.write(store)
}

func (r AssetRegistry) UpsertSkill(skill SkillAsset) error {
	lock := lockForPath(r.path)
	lock.Lock()
	defer lock.Unlock()

	store, err := r.load()
	if err != nil {
		return err
	}
	skill = normalizeSkillAsset(skill, "")
	for index, existing := range store.Skills {
		if existing.ID == skill.ID || (strings.EqualFold(existing.Name, skill.Name) && existing.Version == skill.Version) {
			store.Skills[index] = skill
			return r.write(store)
		}
	}
	store.Skills = append(store.Skills, skill)
	return r.write(store)
}

func (r AssetRegistry) SaveAgentVersion(agent AgentAsset, parentID string) (AgentAsset, error) {
	lock := lockForPath(r.path)
	lock.Lock()
	defer lock.Unlock()

	store, err := r.load()
	if err != nil {
		return AgentAsset{}, err
	}
	agent = normalizeAgentAsset(agent, "")
	nextVersion := 1
	for index, existing := range store.Agents {
		if !strings.EqualFold(existing.Name, agent.Name) {
			continue
		}
		if existing.Version >= nextVersion {
			nextVersion = existing.Version + 1
		}
		if existing.ID == parentID && store.Agents[index].Status == AssetStatusActive {
			store.Agents[index].Status = AssetStatusSuperseded
			store.Agents[index].Lineage.UpdatedAt = time.Now().UTC()
		}
	}

	agent.Version = nextVersion
	agent.ID = ""
	agent = normalizeAgentAsset(agent, parentID)
	store.Agents = append(store.Agents, agent)
	if err := r.write(store); err != nil {
		return AgentAsset{}, err
	}
	return agent, nil
}

func (r AssetRegistry) RecordOutcome(outcome AssetOutcome) error {
	lock := lockForPath(r.path)
	lock.Lock()
	defer lock.Unlock()

	store, err := r.load()
	if err != nil {
		return err
	}
	changed := false
	for index, agent := range store.Agents {
		if agent.ID != outcome.AssetID || (outcome.Kind != "" && outcome.Kind != AssetKindAgent) {
			continue
		}
		store.Agents[index].Metrics = updateAssetMetrics(agent.Metrics, outcome)
		store.Agents[index].Status = statusFromMetrics(store.Agents[index].Status, store.Agents[index].Metrics)
		store.Agents[index].Lineage.UpdatedAt = updatedAt(outcome.CompletedAt)
		if outcome.Lesson != "" {
			store.Agents[index].Lessons = append(store.Agents[index].Lessons, Lesson{
				RunID:     outcome.RunID,
				Summary:   outcome.Lesson,
				CreatedAt: updatedAt(outcome.CompletedAt),
			})
		}
		changed = true
	}
	for index, skill := range store.Skills {
		if skill.ID != outcome.AssetID || (outcome.Kind != "" && outcome.Kind != AssetKindSkill) {
			continue
		}
		store.Skills[index].Metrics = updateAssetMetrics(skill.Metrics, outcome)
		store.Skills[index].Status = statusFromMetrics(store.Skills[index].Status, store.Skills[index].Metrics)
		store.Skills[index].Lineage.UpdatedAt = updatedAt(outcome.CompletedAt)
		if outcome.Lesson != "" {
			store.Skills[index].Lessons = append(store.Skills[index].Lessons, Lesson{
				RunID:     outcome.RunID,
				Summary:   outcome.Lesson,
				CreatedAt: updatedAt(outcome.CompletedAt),
			})
		}
		changed = true
	}
	if !changed {
		return fmt.Errorf("asset not found: %s", outcome.AssetID)
	}
	return r.write(store)
}

func (r AssetRegistry) load() (assetRegistryFile, error) {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return assetRegistryFile{}, err
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyAssetRegistryFile(), nil
		}
		return assetRegistryFile{}, err
	}
	if len(data) == 0 {
		return emptyAssetRegistryFile(), nil
	}

	var store assetRegistryFile
	if err := json.Unmarshal(data, &store); err != nil {
		return assetRegistryFile{}, err
	}
	if store.Agents == nil {
		store.Agents = []AgentAsset{}
	}
	if store.Skills == nil {
		store.Skills = []SkillAsset{}
	}
	return store, nil
}

func (r AssetRegistry) write(store assetRegistryFile) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	sortAgentAssets(store.Agents)
	sortSkillAssets(store.Skills)
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(r.path, data, 0o644)
}

func normalizeAgentAsset(agent AgentAsset, parentID string) AgentAsset {
	now := time.Now().UTC()
	if agent.Name == "" {
		agent.Name = agent.Contract.Name
	}
	if agent.Role == "" {
		agent.Role = agent.Contract.Spec.Role
	}
	if agent.Version == 0 {
		agent.Version = 1
	}
	if agent.Status == "" {
		agent.Status = AssetStatusActive
	}
	if agent.ID == "" {
		agent.ID = assetID(AssetKindAgent, agent.Name, agent.Version)
	}
	if agent.Lineage.CreatedAt.IsZero() {
		agent.Lineage.CreatedAt = now
	}
	if agent.Lineage.UpdatedAt.IsZero() {
		agent.Lineage.UpdatedAt = agent.Lineage.CreatedAt
	}
	if parentID != "" {
		agent.Lineage.ParentID = parentID
		agent.Lineage.SupersedesIDs = mergeStrings(agent.Lineage.SupersedesIDs, []string{parentID})
	}
	return agent
}

func normalizeSkillAsset(skill SkillAsset, parentID string) SkillAsset {
	now := time.Now().UTC()
	if skill.Name == "" {
		skill.Name = skill.Contract.Name
	}
	if skill.Version == 0 {
		skill.Version = 1
	}
	if skill.Status == "" {
		skill.Status = AssetStatusActive
	}
	if skill.ID == "" {
		skill.ID = assetID(AssetKindSkill, skill.Name, skill.Version)
	}
	if skill.Lineage.CreatedAt.IsZero() {
		skill.Lineage.CreatedAt = now
	}
	if skill.Lineage.UpdatedAt.IsZero() {
		skill.Lineage.UpdatedAt = skill.Lineage.CreatedAt
	}
	if parentID != "" {
		skill.Lineage.ParentID = parentID
		skill.Lineage.SupersedesIDs = mergeStrings(skill.Lineage.SupersedesIDs, []string{parentID})
	}
	return skill
}

func assetID(kind string, name string, version int) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = strings.ReplaceAll(normalized, "_", "-")
	if normalized == "" {
		normalized = "unnamed"
	}
	return fmt.Sprintf("%s:%s:v%d", kind, normalized, version)
}

func statusFromMetrics(current string, metrics AssetMetrics) string {
	if current == AssetStatusArchived || current == AssetStatusDeprecated || current == AssetStatusSuperseded {
		return current
	}
	if metrics.TotalRuns >= 3 && metrics.SuccessRate < 0.5 {
		return AssetStatusNeedsReview
	}
	return AssetStatusActive
}

func emptyAssetRegistryFile() assetRegistryFile {
	return assetRegistryFile{
		Agents: []AgentAsset{},
		Skills: []SkillAsset{},
	}
}

func sortAgentAssets(agents []AgentAsset) {
	sort.SliceStable(agents, func(i, j int) bool {
		if strings.EqualFold(agents[i].Name, agents[j].Name) {
			return agents[i].Version > agents[j].Version
		}
		return strings.ToLower(agents[i].Name) < strings.ToLower(agents[j].Name)
	})
}

func sortSkillAssets(skills []SkillAsset) {
	sort.SliceStable(skills, func(i, j int) bool {
		if strings.EqualFold(skills[i].Name, skills[j].Name) {
			return skills[i].Version > skills[j].Version
		}
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})
}

func updatedAt(completedAt time.Time) time.Time {
	if completedAt.IsZero() {
		return time.Now().UTC()
	}
	return completedAt
}
