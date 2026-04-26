package verdandi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Orchestrator struct {
	dataDir string
}

func NewOrchestrator(dataDir string) Orchestrator {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	return Orchestrator{dataDir: dataDir}
}

type stagePattern struct {
	keywords []string
	stage    string
	order    int
}

var stagePatterns = []stagePattern{
	{keywords: []string{"기획", "계획", "설계", "분석", "요구사항"}, stage: "planner", order: stageOrder["planner"]},
	{keywords: []string{"만들어", "구현", "코드", "작성", "개발", "생성"}, stage: "code-writer", order: stageOrder["code-writer"]},
	{keywords: []string{"테스트", "검증", "확인", "실행"}, stage: "tester", order: stageOrder["tester"]},
	{keywords: []string{"문서", "readme", "설명", "가이드"}, stage: "documenter", order: stageOrder["documenter"]},
	{keywords: []string{"배포", "서버", "런칭"}, stage: "deployer", order: stageOrder["deployer"]},
}

var stageOrder = map[string]int{
	"planner":     1,
	"code-writer": 2,
	"tester":      3,
	"documenter":  4,
	"deployer":    5,
}

func (o Orchestrator) ParseRequest(request string) Plan {
	processed := strings.ToLower(request)
	stages := []StageDef{}
	seen := map[string]bool{}

	for _, pattern := range stagePatterns {
		for _, keyword := range pattern.keywords {
			if strings.Contains(processed, strings.ToLower(keyword)) {
				if !seen[pattern.stage] {
					stages = append(stages, StageDef{
						Stage:   pattern.stage,
						Keyword: keyword,
						Order:   pattern.order,
					})
					seen[pattern.stage] = true
				}
				break
			}
		}
	}

	plan, err := o.NormalizePlan(request, stages)
	if err != nil {
		fallback := []StageDef{{Stage: "code-writer", Keyword: "default", Order: stageOrder["code-writer"]}}
		plan := Plan{
			OriginalRequest: request,
			Stages:          fallback,
			StageCount:      len(fallback),
			Graph:           buildGraph(fallback),
		}
		return plan
	}
	return plan
}

func (o Orchestrator) NormalizePlan(request string, proposed []StageDef) (Plan, error) {
	stages := []StageDef{}
	seen := map[string]bool{}

	for _, stage := range proposed {
		if !isAllowedStage(stage.Stage) {
			return Plan{}, fmt.Errorf("unknown stage from analyzer: %s", stage.Stage)
		}
		if seen[stage.Stage] {
			continue
		}
		keyword := stage.Keyword
		if keyword == "" {
			keyword = "analyzer"
		}
		stages = append(stages, StageDef{
			Stage:   stage.Stage,
			Keyword: keyword,
			Order:   stageOrder[stage.Stage],
			Agent:   normalizeAgentContract(request, stage.Stage, stage.Agent),
		})
		seen[stage.Stage] = true
	}

	if len(stages) == 0 {
		stages = append(stages, StageDef{Stage: "code-writer", Keyword: "default", Order: stageOrder["code-writer"]})
		seen["code-writer"] = true
	}

	if seen["code-writer"] && !seen["tester"] {
		stages = append(stages, StageDef{Stage: "tester", Keyword: "auto-validation", Order: stageOrder["tester"]})
		seen["tester"] = true
	}

	sort.SliceStable(stages, func(i, j int) bool {
		return stages[i].Order < stages[j].Order
	})

	plan := Plan{
		OriginalRequest: request,
		Stages:          stages,
		StageCount:      len(stages),
	}
	plan.Graph = buildGraph(stages)
	return plan, nil
}

func isAllowedStage(stage string) bool {
	_, ok := stageOrder[stage]
	return ok
}

func normalizeAgentContract(request string, stage string, agent *AgentContract) *AgentContract {
	if agent == nil {
		return nil
	}
	normalized := *agent
	if normalized.Name == "" {
		normalized.Name = agentName(stage)
	}
	if normalized.Description == "" {
		normalized.Description = "Dynamic Verdandi agent contract"
	}
	if normalized.Command == "" {
		normalized.Command = "verdandi"
	}
	if normalized.Spec.Role == "" {
		normalized.Spec.Role = stage
	}
	if len(normalized.Spec.Capabilities) == 0 {
		normalized.Spec.Capabilities = capabilitiesFor(stage)
	}
	if normalized.Metadata == nil {
		normalized.Metadata = map[string]any{}
	}
	normalized.Metadata["executorStage"] = stage
	if normalized.Inputs == nil {
		normalized.Inputs = map[string]string{}
	}
	if normalized.Inputs["request"] == "" {
		normalized.Inputs["request"] = request
	}
	return &normalized
}

func (o Orchestrator) Execute(request string, options map[string]any) (ExecutionResult, error) {
	plan := o.ParseRequest(request)
	return o.ExecutePlan(plan, options)
}

func (o Orchestrator) ExecutePlan(plan Plan, options map[string]any) (ExecutionResult, error) {
	result := ExecutionResult{
		Request: plan.OriginalRequest,
		Plan:    plan,
		Stages:  []StageResult{},
	}

	var previous *StageOutput
	for _, stage := range plan.Stages {
		started := time.Now().UTC()
		stageOutput, err := o.executeStage(stage.Stage, plan.OriginalRequest, previous)
		record := StageResult{
			Stage:   stage.Stage,
			Started: started,
			Ended:   time.Now().UTC(),
		}
		if err != nil {
			record.Status = "error"
			record.Error = err.Error()
			result.Stages = append(result.Stages, record)
			if stopOnError(options) {
				break
			}
			continue
		}

		record.Status = "success"
		record.Result = &stageOutput
		result.Stages = append(result.Stages, record)
		previous = &stageOutput
		if stageOutput.OutputDir != "" {
			result.OutputDir = stageOutput.OutputDir
		}
	}

	result.Summary = summarize(result)
	result.CompletedAt = time.Now().UTC()
	return result, nil
}

func (o Orchestrator) executeStage(stage string, request string, previous *StageOutput) (StageOutput, error) {
	switch stage {
	case "planner":
		return o.writeFiles(previousOutputDir(previous), []generatedFile{{
			Name:    "requirements.md",
			Content: generatePlan(request),
		}})
	case "code-writer":
		return o.writeFiles(previousOutputDir(previous), generateCode(request))
	case "tester":
		outputDir := previousOutputDir(previous)
		if outputDir == "" {
			return StageOutput{}, fmt.Errorf("테스트할 코드가 없습니다")
		}
		return runValidation(outputDir)
	case "documenter":
		outputDir := previousOutputDir(previous)
		files, err := listOutputFiles(outputDir)
		if err != nil {
			return StageOutput{}, err
		}
		return o.writeFiles(outputDir, []generatedFile{{
			Name:    "README.md",
			Content: generateDocumentation(request, files),
		}})
	case "deployer":
		return o.writeFiles(previousOutputDir(previous), []generatedFile{{
			Name:    "deploy.sh",
			Content: "#!/bin/sh\nset -eu\ngo test ./...\n",
			Mode:    0o755,
		}})
	default:
		return StageOutput{}, fmt.Errorf("unknown stage: %s", stage)
	}
}

type generatedFile struct {
	Name    string
	Content string
	Mode    os.FileMode
}

func (o Orchestrator) writeFiles(outputDir string, files []generatedFile) (StageOutput, error) {
	if outputDir == "" {
		outputDir = filepath.Join(o.dataDir, "output", fmt.Sprintf("project_%s_%d", time.Now().UTC().Format("2006-01-02"), time.Now().UnixNano()))
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return StageOutput{}, err
	}

	entries := []FileEntry{}
	for _, file := range files {
		mode := file.Mode
		if mode == 0 {
			mode = 0o644
		}
		filePath := filepath.Join(outputDir, file.Name)
		if err := os.WriteFile(filePath, []byte(file.Content), mode); err != nil {
			return StageOutput{}, err
		}
		stat, err := os.Stat(filePath)
		if err != nil {
			return StageOutput{}, err
		}
		entries = append(entries, FileEntry{
			Name:   file.Name,
			Path:   filePath,
			Size:   stat.Size(),
			Status: "success",
		})
	}

	return StageOutput{
		Type:      "files",
		Files:     entries,
		OutputDir: outputDir,
	}, nil
}

func buildGraph(stages []StageDef) ExecutionGraph {
	nodes := make([]string, 0, len(stages))
	seen := map[string]bool{}
	for _, stage := range stages {
		nodes = append(nodes, stage.Stage)
		seen[stage.Stage] = true
	}

	edges := []Edge{}
	add := func(from, to string) {
		if seen[from] && seen[to] {
			for _, edge := range edges {
				if edge.From == from && edge.To == to {
					return
				}
			}
			edges = append(edges, Edge{From: from, To: to})
		}
	}

	add("planner", "code-writer")
	add("code-writer", "tester")
	if seen["tester"] {
		add("tester", "documenter")
		add("tester", "deployer")
	} else {
		add("code-writer", "documenter")
		add("code-writer", "deployer")
	}
	add("documenter", "deployer")

	return ExecutionGraph{Nodes: nodes, Edges: edges}
}

func summarize(result ExecutionResult) Summary {
	summary := Summary{TotalStages: len(result.Stages), OutputDir: result.OutputDir, Files: []FileEntry{}}
	for _, stage := range result.Stages {
		if stage.Status == "success" {
			summary.Success++
			if stage.Result != nil {
				summary.Files = append(summary.Files, stage.Result.Files...)
			}
		} else {
			summary.Failed++
		}
	}
	return summary
}

func previousOutputDir(previous *StageOutput) string {
	if previous == nil {
		return ""
	}
	return previous.OutputDir
}

func stopOnError(options map[string]any) bool {
	if options == nil {
		return false
	}
	value, _ := options["stopOnError"].(bool)
	return value
}

func generateCode(request string) []generatedFile {
	moduleName := moduleNameFromRequest(request)
	return []generatedFile{
		{
			Name: "go.mod",
			Content: fmt.Sprintf(`module %s

go 1.22
`, moduleName),
		},
		{
			Name: "main.go",
			Content: fmt.Sprintf(`package main

import "fmt"

const request = %q

func main() {
	fmt.Println("Verdandi generated project")
	fmt.Println(request)
}
`, request),
		},
		{
			Name: "main_test.go",
			Content: `package main

import "testing"

func TestRequestIsCaptured(t *testing.T) {
	if request == "" {
		t.Fatal("request should be captured")
	}
}
`,
		},
	}
}

func runValidation(outputDir string) (StageOutput, error) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return StageOutput{}, err
	}
	foundGo := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".go") {
			foundGo = true
			break
		}
	}
	if !foundGo {
		return StageOutput{}, fmt.Errorf("검증할 Go 파일이 없습니다")
	}

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = outputDir
	output, err := cmd.CombinedOutput()
	testOutput := strings.TrimSpace(string(output))
	if err != nil {
		return StageOutput{}, fmt.Errorf("go test failed: %s", testOutput)
	}

	return StageOutput{
		Type:      "test",
		Status:    "success",
		OutputDir: outputDir,
		Tests: []TestResult{{
			Name:   "go-test",
			Status: "success",
			Output: testOutput,
		}},
	}, nil
}

func generatePlan(request string) string {
	return fmt.Sprintf(`# Requirements

## Goal
Create a local generated project for this request:

%s

## Scope
- Parse the request into an ordered Verdandi workflow.
- Generate project files in a local output directory.
- Validate the generated Go project before reporting success.

## Workflow
1. Analyze the request.
2. Select the required stages.
3. Execute each stage with prior output as context.
4. Validate the generated result.
5. Record the run for later status and output lookup.

## Acceptance Criteria
- The generated project contains a Go module.
- The generated project passes `+"`go test ./...`"+`.
- The run summary lists generated files and stage outcomes.
`, request)
}

func generateDocumentation(request string, files []FileEntry) string {
	names := listFileNames(files)
	var builder strings.Builder
	builder.WriteString("# Generated Project\n\n")
	builder.WriteString("## Request\n")
	builder.WriteString(request)
	builder.WriteString("\n\n## Files\n")
	for _, name := range names {
		builder.WriteString("- `")
		builder.WriteString(name)
		builder.WriteString("`\n")
	}
	builder.WriteString("\n## Run\n\n")
	builder.WriteString("```bash\n")
	builder.WriteString("go run .\n")
	builder.WriteString("```\n\n")
	builder.WriteString("## Test\n\n")
	builder.WriteString("```bash\n")
	builder.WriteString("go test ./...\n")
	builder.WriteString("```\n")
	return builder.String()
}

func moduleNameFromRequest(request string) string {
	base := "generated-project"
	processed := preprocess(request)
	if processed == "" {
		return base
	}
	parts := strings.Fields(processed)
	if len(parts) == 0 {
		return base
	}
	candidate := parts[0]
	if containsKorean(candidate) {
		return base
	}
	candidate = strings.Trim(candidate, "-_")
	if candidate == "" {
		return base
	}
	return candidate
}

func listOutputFiles(outputDir string) ([]FileEntry, error) {
	if outputDir == "" {
		return nil, fmt.Errorf("문서화할 출력 디렉터리가 없습니다")
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, err
	}
	files := []FileEntry{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, FileEntry{
			Name:   entry.Name(),
			Path:   filepath.Join(outputDir, entry.Name()),
			Size:   info.Size(),
			Status: "success",
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})
	return files, nil
}

func listFileNames(entries []FileEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	sort.Strings(names)
	return names
}
