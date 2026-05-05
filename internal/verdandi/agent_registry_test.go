package verdandi

import "testing"

func TestFindSimilarAgentDoesNotOvermatchGenericSmallAgent(t *testing.T) {
	agents := []AgentContract{{
		Name:        "GeneralFrontendAgent",
		Description: "Builds interfaces.",
		Spec: AgentSpec{
			Role:         "frontend engineer",
			Capabilities: []string{"ui"},
		},
	}}
	candidate := AgentContract{
		Name:        "AccessibilityDashboardSecurityAgent",
		Description: "Builds accessible dashboards with security auditing and data visualization.",
		Spec: AgentSpec{
			Role: "frontend accessibility security dashboard engineer",
			Capabilities: []string{
				"ui",
				"accessibility",
				"dashboard",
				"security",
				"data-visualization",
			},
		},
	}

	index, similarity := findSimilarAgent(agents, candidate)
	if index >= 0 {
		t.Fatalf("expected generic agent not to match, got index %d with similarity %.2f", index, similarity)
	}
}
