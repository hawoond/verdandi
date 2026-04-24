package verdandi

import (
	"fmt"
	"os"
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
	{keywords: []string{"기획", "계획", "설계", "분석", "요구사항"}, stage: "planner", order: 1},
	{keywords: []string{"만들어", "구현", "코드", "작성", "개발", "생성"}, stage: "code-writer", order: 2},
	{keywords: []string{"테스트", "검증", "확인", "실행"}, stage: "tester", order: 3},
	{keywords: []string{"문서", "readme", "설명", "가이드"}, stage: "documenter", order: 4},
	{keywords: []string{"배포", "서버", "런칭"}, stage: "deployer", order: 5},
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

	if len(stages) == 0 {
		stages = append(stages, StageDef{Stage: "code-writer", Keyword: "default", Order: 1})
		seen["code-writer"] = true
	}

	if seen["code-writer"] && !seen["tester"] {
		stages = append(stages, StageDef{Stage: "tester", Keyword: "auto-validation", Order: 99})
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
	return plan
}

func (o Orchestrator) Execute(request string, options map[string]any) (ExecutionResult, error) {
	plan := o.ParseRequest(request)
	result := ExecutionResult{
		Request: request,
		Plan:    plan,
		Stages:  []StageResult{},
	}

	var previous map[string]any
	for _, stage := range plan.Stages {
		started := time.Now().UTC()
		stageResult, err := o.executeStage(stage.Stage, request, previous)
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
		record.Result = stageResult
		result.Stages = append(result.Stages, record)
		previous = stageResult
		if outputDir, ok := stageResult["outputDir"].(string); ok && outputDir != "" {
			result.OutputDir = outputDir
		}
	}

	result.Summary = summarize(result)
	result.CompletedAt = time.Now().UTC()
	return result, nil
}

func (o Orchestrator) executeStage(stage string, request string, previous map[string]any) (map[string]any, error) {
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
			return nil, fmt.Errorf("테스트할 코드가 없습니다")
		}
		return runValidation(outputDir)
	case "documenter":
		return o.writeFiles(previousOutputDir(previous), []generatedFile{{
			Name:    "README.md",
			Content: generateDocumentation(request),
		}})
	case "deployer":
		return o.writeFiles(previousOutputDir(previous), []generatedFile{{
			Name:    "deploy.sh",
			Content: "#!/bin/sh\nset -eu\ngo test ./...\n",
			Mode:    0o755,
		}})
	default:
		return nil, fmt.Errorf("unknown stage: %s", stage)
	}
}

type generatedFile struct {
	Name    string
	Content string
	Mode    os.FileMode
}

func (o Orchestrator) writeFiles(outputDir string, files []generatedFile) (map[string]any, error) {
	if outputDir == "" {
		outputDir = filepath.Join(o.dataDir, "output", fmt.Sprintf("project_%s_%d", time.Now().UTC().Format("2006-01-02"), time.Now().UnixNano()))
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}

	entries := []FileEntry{}
	for _, file := range files {
		mode := file.Mode
		if mode == 0 {
			mode = 0o644
		}
		filePath := filepath.Join(outputDir, file.Name)
		if err := os.WriteFile(filePath, []byte(file.Content), mode); err != nil {
			return nil, err
		}
		stat, err := os.Stat(filePath)
		if err != nil {
			return nil, err
		}
		entries = append(entries, FileEntry{
			Name:   file.Name,
			Path:   filePath,
			Size:   stat.Size(),
			Status: "success",
		})
	}

	return map[string]any{
		"type":      "files",
		"files":     entries,
		"outputDir": outputDir,
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
			if files, ok := stage.Result["files"].([]FileEntry); ok {
				summary.Files = append(summary.Files, files...)
			}
		} else {
			summary.Failed++
		}
	}
	return summary
}

func previousOutputDir(previous map[string]any) string {
	if previous == nil {
		return ""
	}
	outputDir, _ := previous["outputDir"].(string)
	return outputDir
}

func stopOnError(options map[string]any) bool {
	if options == nil {
		return false
	}
	value, _ := options["stopOnError"].(bool)
	return value
}

func generateCode(request string) []generatedFile {
	return []generatedFile{
		{
			Name: "main.go",
			Content: fmt.Sprintf(`package main

import "fmt"

func main() {
	fmt.Println(%q)
}
`, request),
		},
		{
			Name: "main_test.go",
			Content: `package main

import "testing"

func TestGeneratedProgram(t *testing.T) {
	t.Helper()
}
`,
		},
	}
}

func runValidation(outputDir string) (map[string]any, error) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, err
	}
	foundGo := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".go") {
			foundGo = true
			break
		}
	}
	if !foundGo {
		return nil, fmt.Errorf("검증할 Go 파일이 없습니다")
	}
	return map[string]any{
		"type":      "test",
		"status":    "success",
		"outputDir": outputDir,
		"tests": []map[string]any{{
			"name":   "static-go-file-check",
			"status": "success",
		}},
	}, nil
}

func generatePlan(request string) string {
	return fmt.Sprintf(`# Requirements

## Overview
%s

## Workflow
1. Analyze the request.
2. Select the required agents.
3. Execute each stage with prior output as context.
4. Validate the generated result.
`, request)
}

func generateDocumentation(request string) string {
	return fmt.Sprintf(`# Generated Project

This project was generated from:

%s

## Run

`+"```bash\n"+`go run .
`+"```\n", request)
}
