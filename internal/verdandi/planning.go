package verdandi

import (
	"fmt"
	"strings"
)

type PlanningOutput struct {
	Request            string
	Goal               string
	FunctionalReqs     []string
	NonGoals           []string
	AcceptanceCriteria []string
	QualityGates       []string
	Tasks              []string
	SuggestedOrder     []string
	Assumptions        []string
	OpenQuestions      []string
	Risks              []string
}

func BuildPlanningOutput(request string) PlanningOutput {
	goal := strings.TrimSpace(request)
	if goal == "" {
		goal = "Create a local generated project from the user request."
	}
	return PlanningOutput{
		Request: goal,
		Goal:    fmt.Sprintf("Deliver a small, inspectable Verdandi workflow for: %s", goal),
		FunctionalReqs: []string{
			"Parse the request into an ordered Verdandi workflow.",
			"Generate local project files in a reproducible output directory.",
			"Validate generated Go code before reporting success.",
			"Record stage results so status and output lookup stay inspectable.",
		},
		NonGoals: []string{
			"Do not execute arbitrary shell commands from the request.",
			"Do not call external services during the local MVP workflow.",
		},
		AcceptanceCriteria: []string{
			"The generated project contains a Go module.",
			"The generated project passes `go test ./...`.",
			"The run summary lists generated files and stage outcomes.",
		},
		QualityGates: []string{
			"Planner output is written before implementation artifacts.",
			"Tester stage runs after code generation when code is produced.",
			"Failed stages are represented in the run summary.",
		},
		Tasks: []string{
			"Confirm the requested outcome and scope.",
			"Generate implementation files for the requested project.",
			"Run validation against the generated Go module.",
			"Document the generated files and run commands.",
		},
		SuggestedOrder: []string{
			"planner",
			"code-writer",
			"tester",
			"documenter",
		},
		Assumptions: []string{
			"The request can be represented as a local Go project.",
			"The local Go toolchain is available for validation.",
		},
		OpenQuestions: []string{
			"What user-facing behavior should be prioritized beyond the generated scaffold?",
			"Are there domain constraints that should change the generated module shape?",
		},
		Risks: []string{
			"Keyword-based planning may miss nuanced user intent.",
			"Generated scaffolds may be too generic for production use without follow-up refinement.",
		},
	}
}

func planningFiles(request string) []generatedFile {
	output := BuildPlanningOutput(request)
	return []generatedFile{
		{Name: "requirements.md", Content: RenderRequirements(output)},
		{Name: "acceptance-criteria.md", Content: RenderAcceptanceCriteria(output)},
		{Name: "task-breakdown.md", Content: RenderTaskBreakdown(output)},
		{Name: "risks.md", Content: RenderRisks(output)},
	}
}

func RenderRequirements(output PlanningOutput) string {
	var builder strings.Builder
	builder.WriteString("# Requirements\n\n")
	builder.WriteString("## Goal\n")
	builder.WriteString(output.Goal)
	builder.WriteString("\n\n## Functional Requirements\n")
	writeMarkdownList(&builder, output.FunctionalReqs)
	builder.WriteString("\n## Non-Goals\n")
	writeMarkdownList(&builder, output.NonGoals)
	return builder.String()
}

func RenderAcceptanceCriteria(output PlanningOutput) string {
	var builder strings.Builder
	builder.WriteString("# Acceptance Criteria\n\n")
	builder.WriteString("## Functional Validation\n")
	writeMarkdownList(&builder, output.AcceptanceCriteria)
	builder.WriteString("\n## Quality Gates\n")
	writeMarkdownList(&builder, output.QualityGates)
	return builder.String()
}

func RenderTaskBreakdown(output PlanningOutput) string {
	var builder strings.Builder
	builder.WriteString("# Task Breakdown\n\n")
	builder.WriteString("## Implementation Tasks\n")
	writeMarkdownList(&builder, output.Tasks)
	builder.WriteString("\n## Suggested Order\n")
	writeMarkdownList(&builder, output.SuggestedOrder)
	return builder.String()
}

func RenderRisks(output PlanningOutput) string {
	var builder strings.Builder
	builder.WriteString("# Risks\n\n")
	builder.WriteString("## Assumptions\n")
	writeMarkdownList(&builder, output.Assumptions)
	builder.WriteString("\n## Open Questions\n")
	writeMarkdownList(&builder, output.OpenQuestions)
	builder.WriteString("\n## Risks\n")
	writeMarkdownList(&builder, output.Risks)
	return builder.String()
}

func writeMarkdownList(builder *strings.Builder, values []string) {
	for _, value := range values {
		builder.WriteString("- ")
		builder.WriteString(value)
		builder.WriteString("\n")
	}
}
