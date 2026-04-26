package verdandi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type AgentRegistry struct {
	path string
}

type agentRegistryFile struct {
	Agents []AgentContract `json:"agents"`
}

func NewAgentRegistry(path string) AgentRegistry {
	return AgentRegistry{path: path}
}

func (r AgentRegistry) ResolvePlan(plan Plan, options map[string]any) (Plan, error) {
	store, err := r.load()
	if err != nil {
		return Plan{}, err
	}

	defaultPolicy := agentPolicy(options)
	changed := false
	for index, stage := range plan.Stages {
		if stage.Agent == nil {
			continue
		}

		candidate := cloneAgent(*stage.Agent)
		policy := defaultPolicy
		if candidatePolicy := agentPolicyFromMetadata(candidate.Metadata); candidatePolicy != "" {
			policy = candidatePolicy
		}

		existingIndex, similarity := findSimilarAgent(store.Agents, candidate)
		hasSimilar := existingIndex >= 0
		action := policy
		if !hasSimilar {
			action = AgentPolicySeparate
		}

		decision := AgentLifecycleDecision{
			Action:     action,
			Reason:     agentDecisionReason(action, hasSimilar),
			Similarity: similarity,
			Options:    agentPolicyOptions(),
		}
		if hasSimilar {
			decision.ExistingAgentName = store.Agents[existingIndex].Name
		}

		switch action {
		case AgentPolicyReuseEnhance:
			merged := mergeAgents(store.Agents[existingIndex], candidate)
			store.Agents[existingIndex] = merged
			plan.Stages[index].Agent = &merged
		case AgentPolicyRewrite:
			store.Agents[existingIndex] = candidate
			plan.Stages[index].Agent = &candidate
		case AgentPolicySeparate:
			upsertAgent(&store.Agents, candidate)
			plan.Stages[index].Agent = &candidate
		default:
			return Plan{}, fmt.Errorf("unknown agent policy: %s", action)
		}
		plan.Stages[index].AgentDecision = &decision
		changed = true
	}

	if !changed {
		return plan, nil
	}
	if err := r.write(store); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (r AgentRegistry) load() (agentRegistryFile, error) {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return agentRegistryFile{}, err
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return agentRegistryFile{Agents: []AgentContract{}}, nil
		}
		return agentRegistryFile{}, err
	}
	if len(data) == 0 {
		return agentRegistryFile{Agents: []AgentContract{}}, nil
	}

	var store agentRegistryFile
	if err := json.Unmarshal(data, &store); err != nil {
		return agentRegistryFile{}, err
	}
	if store.Agents == nil {
		store.Agents = []AgentContract{}
	}
	return store, nil
}

func (r AgentRegistry) write(store agentRegistryFile) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	sort.SliceStable(store.Agents, func(i, j int) bool {
		return strings.ToLower(store.Agents[i].Name) < strings.ToLower(store.Agents[j].Name)
	})
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0o644)
}

func agentPolicy(options map[string]any) string {
	if options != nil {
		for _, key := range []string{"agentPolicy", "agentLifecyclePolicy"} {
			if value, ok := options[key].(string); ok && isAgentPolicy(value) {
				return value
			}
		}
	}
	return AgentPolicyReuseEnhance
}

func agentPolicyFromMetadata(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	for _, key := range []string{"agentPolicy", "agentLifecyclePolicy"} {
		if value, ok := metadata[key].(string); ok && isAgentPolicy(value) {
			return value
		}
	}
	return ""
}

func isAgentPolicy(value string) bool {
	switch value {
	case AgentPolicyReuseEnhance, AgentPolicyRewrite, AgentPolicySeparate:
		return true
	default:
		return false
	}
}

func agentPolicyOptions() []string {
	return []string{AgentPolicyReuseEnhance, AgentPolicyRewrite, AgentPolicySeparate}
}

func agentDecisionReason(action string, hasSimilar bool) string {
	if !hasSimilar {
		return "no similar existing agent found"
	}
	switch action {
	case AgentPolicyReuseEnhance:
		return "similar existing agent found; reusing and enhancing it"
	case AgentPolicyRewrite:
		return "similar existing agent found; replacing it with the candidate"
	case AgentPolicySeparate:
		return "similar existing agent found; keeping the candidate separate"
	default:
		return "agent lifecycle policy selected"
	}
}

func findSimilarAgent(agents []AgentContract, candidate AgentContract) (int, float64) {
	bestIndex := -1
	bestScore := 0.0
	for index, agent := range agents {
		score := agentSimilarity(agent, candidate)
		if score > bestScore {
			bestIndex = index
			bestScore = score
		}
	}
	if bestScore < 0.34 {
		return -1, bestScore
	}
	return bestIndex, bestScore
}

func agentSimilarity(a AgentContract, b AgentContract) float64 {
	left := agentTerms(a)
	right := agentTerms(b)
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	common := 0
	for term := range left {
		if right[term] {
			common++
		}
	}
	denominator := len(left)
	if len(right) < denominator {
		denominator = len(right)
	}
	return float64(common) / float64(denominator)
}

func agentTerms(agent AgentContract) map[string]bool {
	terms := map[string]bool{}
	addTerms(terms, agent.Name)
	addTerms(terms, agent.Description)
	addTerms(terms, agent.Spec.Role)
	for _, capability := range agent.Spec.Capabilities {
		addTerms(terms, capability)
	}
	return terms
}

func addTerms(terms map[string]bool, text string) {
	for _, token := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '.' || r == ',' || r == '/' || r == ':'
	}) {
		if token != "" {
			terms[token] = true
		}
	}
}

func mergeAgents(existing AgentContract, candidate AgentContract) AgentContract {
	merged := cloneAgent(existing)
	if candidate.Description != "" && !strings.Contains(strings.ToLower(merged.Description), strings.ToLower(candidate.Description)) {
		if merged.Description == "" {
			merged.Description = candidate.Description
		} else {
			merged.Description = merged.Description + " " + candidate.Description
		}
	}
	if merged.Command == "" {
		merged.Command = candidate.Command
	}
	if merged.Spec.Role == "" {
		merged.Spec.Role = candidate.Spec.Role
	}
	merged.Spec.Capabilities = mergeStrings(merged.Spec.Capabilities, candidate.Spec.Capabilities)
	if merged.Metadata == nil {
		merged.Metadata = map[string]any{}
	}
	for key, value := range candidate.Metadata {
		merged.Metadata[key] = value
	}
	if merged.Inputs == nil {
		merged.Inputs = map[string]string{}
	}
	for key, value := range candidate.Inputs {
		merged.Inputs[key] = value
	}
	return merged
}

func mergeStrings(left []string, right []string) []string {
	seen := map[string]bool{}
	merged := []string{}
	for _, value := range append(left, right...) {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		merged = append(merged, value)
	}
	return merged
}

func upsertAgent(agents *[]AgentContract, candidate AgentContract) {
	for index, agent := range *agents {
		if strings.EqualFold(agent.Name, candidate.Name) {
			(*agents)[index] = candidate
			return
		}
	}
	*agents = append(*agents, candidate)
}

func cloneAgent(agent AgentContract) AgentContract {
	clone := agent
	clone.Spec.Capabilities = append([]string{}, agent.Spec.Capabilities...)
	if agent.Metadata != nil {
		clone.Metadata = map[string]any{}
		for key, value := range agent.Metadata {
			clone.Metadata[key] = value
		}
	}
	if agent.Inputs != nil {
		clone.Inputs = map[string]string{}
		for key, value := range agent.Inputs {
			clone.Inputs[key] = value
		}
	}
	return clone
}
