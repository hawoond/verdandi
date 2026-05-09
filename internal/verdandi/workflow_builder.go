package verdandi

import (
	"fmt"
	"strings"
	"time"
)

func BuildWorkflowPackageFromPlan(runID string, request string, plan Plan) WorkflowPackage {
	if strings.TrimSpace(request) == "" {
		request = plan.OriginalRequest
	}
	if strings.TrimSpace(runID) == "" {
		runID = createRunID()
	}

	agents := make([]AgentAsset, 0, len(plan.Stages))
	skills := make([]SkillAsset, 0, len(plan.Stages))
	tasks := make([]WorkflowTask, 0, len(plan.Stages))

	for index, stage := range plan.Stages {
		agent := normalizeAgentAsset(assetAgentForStage(stage), "")
		skill := normalizeSkillAsset(assetSkillForStage(stage), "")
		agent.Skills = mergeStrings(agent.Skills, []string{skill.ID})
		skill.UsedByAgents = mergeStrings(skill.UsedByAgents, []string{agent.ID})

		agents = append(agents, agent)
		skills = append(skills, skill)
		tasks = append(tasks, WorkflowTask{
			ID:          stageTaskID(index, stage),
			Title:       stageTitle(stage),
			Description: stageDescription(stage),
			AgentID:     agent.ID,
			SkillIDs:    []string{skill.ID},
			DependsOn:   dependenciesForStage(plan, index),
		})
	}

	return WorkflowPackage{
		RunID:              runID,
		Request:            request,
		Agents:             agents,
		Skills:             skills,
		Tasks:              tasks,
		AcceptanceCriteria: acceptanceCriteriaForPlan(plan),
		CreatedAt:          time.Now().UTC(),
	}
}

func assetAgentForStage(stage StageDef) AgentAsset {
	if stage.Agent != nil {
		contract := cloneAgent(*stage.Agent)
		return AgentAsset{
			Name:     contract.Name,
			Role:     stage.Stage,
			Status:   AssetStatusActive,
			Contract: contract,
		}
	}

	title := stageTitle(stage)
	name := "Verdandi" + compactIdentifier(title) + "Agent"
	role := strings.TrimSpace(stage.Stage)
	if role == "" {
		role = IntentGeneral
	}
	return AgentAsset{
		Name:   name,
		Role:   role,
		Status: AssetStatusActive,
		Contract: AgentContract{
			Name:        name,
			Description: fmt.Sprintf("Reusable Verdandi agent asset for %s workflow stages.", strings.ToLower(title)),
			Command:     "codex",
			Spec: AgentSpec{
				Role:         role,
				Capabilities: capabilitiesFor(role),
			},
			Metadata: map[string]any{
				"stage":   stage.Stage,
				"keyword": stage.Keyword,
			},
			Inputs: map[string]string{
				"request": "Original user request",
				"task":    "Workflow task assigned to this agent",
			},
		},
	}
}

func assetSkillForStage(stage StageDef) SkillAsset {
	title := stageTitle(stage)
	stageName := strings.TrimSpace(stage.Stage)
	if stageName == "" {
		stageName = IntentGeneral
	}
	name := strings.ToLower(strings.ReplaceAll(title, " ", "-")) + "-workflow-skill"
	return SkillAsset{
		Name:   name,
		Status: AssetStatusActive,
		Contract: SkillContract{
			Name:         name,
			Description:  fmt.Sprintf("Reusable instructions for %s workflow tasks.", strings.ToLower(title)),
			WhenToUse:    fmt.Sprintf("Use for Verdandi %s stages prepared from a user request.", stageName),
			Instructions: stageInstructions(stageName),
			Inputs:       []string{"request", "workflow task", "selected assets"},
			Outputs:      []string{"implementation notes", "changed files", "verification results"},
			Constraints: []string{
				"Use the external LLM coding agent to perform source changes.",
				"Keep Verdandi output limited to workflow assets, task graph, and handoff guidance.",
			},
			Metadata: map[string]string{
				"stage":   stage.Stage,
				"keyword": stage.Keyword,
			},
		},
	}
}

func stageTitle(stage StageDef) string {
	switch strings.TrimSpace(stage.Stage) {
	case "planner":
		return "Planning"
	case "code-writer":
		return "Implementation"
	case "tester":
		return "Verification"
	case "documenter":
		return "Documentation"
	case "deployer":
		return "Release"
	case "":
		return "General"
	default:
		return titleFromIdentifier(stage.Stage)
	}
}

func stageDescription(stage StageDef) string {
	switch strings.TrimSpace(stage.Stage) {
	case "planner":
		return "Clarify requirements, risks, acceptance criteria, and the implementation task breakdown."
	case "code-writer":
		return "Apply the requested source changes using the selected reusable agent and skill assets."
	case "tester":
		return "Run focused verification and capture the exact test results for the workflow handoff."
	case "documenter":
		return "Update user-facing or developer documentation that should change with the implementation."
	case "deployer":
		return "Prepare release or deployment steps after implementation and verification are complete."
	default:
		if strings.TrimSpace(stage.Stage) == "" {
			return "Handle the prepared workflow task with the selected reusable assets."
		}
		return fmt.Sprintf("Handle the %s workflow task with the selected reusable assets.", stage.Stage)
	}
}

func dependenciesForStage(plan Plan, index int) []string {
	if index <= 0 || index >= len(plan.Stages) {
		return []string{}
	}

	dependencies := []string{}
	stage := plan.Stages[index]
	for _, edge := range plan.Graph.Edges {
		if edge.To != stage.Stage {
			continue
		}
		for previousIndex, previous := range plan.Stages {
			if previous.Stage == edge.From {
				dependencies = append(dependencies, stageTaskID(previousIndex, previous))
			}
		}
	}
	if len(dependencies) > 0 {
		return dependencies
	}
	return []string{stageTaskID(index-1, plan.Stages[index-1])}
}

func stageTaskID(index int, stage StageDef) string {
	name := strings.TrimSpace(stage.Stage)
	if name == "" {
		name = "stage"
	}
	return fmt.Sprintf("%02d-%s", index+1, strings.ToLower(strings.ReplaceAll(name, " ", "-")))
}

func stageInstructions(stage string) string {
	switch stage {
	case "planner":
		return "Analyze the request, identify constraints, produce acceptance criteria, and hand off an ordered implementation plan."
	case "code-writer":
		return "Inspect the repository, make focused source changes, and preserve unrelated user edits."
	case "tester":
		return "Run the smallest meaningful verification first, then broaden only as risk requires. Report exact commands and outcomes."
	case "documenter":
		return "Update documentation to match verified behavior without adding unimplemented promises."
	case "deployer":
		return "Prepare release steps from verified artifacts and call out any remaining manual approvals."
	default:
		return "Complete the assigned workflow task, keep changes scoped, and report verification evidence."
	}
}

func acceptanceCriteriaForPlan(plan Plan) []string {
	criteria := []string{
		"Selected agent and skill assets are available for reuse in future workflows.",
		"The workflow handoff explains that the external LLM coding agent performs source changes.",
	}
	for _, stage := range plan.Stages {
		criteria = append(criteria, fmt.Sprintf("%s task is completed and verified.", stageTitle(stage)))
	}
	return criteria
}

func compactIdentifier(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '.'
	})
	var builder strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		builder.WriteString(strings.ToUpper(string(runes[0])))
		if len(runes) > 1 {
			builder.WriteString(string(runes[1:]))
		}
	}
	if builder.Len() == 0 {
		return "General"
	}
	return builder.String()
}

func titleFromIdentifier(value string) string {
	parts := strings.Fields(strings.ReplaceAll(value, "-", " "))
	for index, part := range parts {
		runes := []rune(part)
		if len(runes) == 0 {
			continue
		}
		parts[index] = strings.ToUpper(string(runes[0])) + string(runes[1:])
	}
	if len(parts) == 0 {
		return "General"
	}
	return strings.Join(parts, " ")
}
