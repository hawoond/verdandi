package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/genie-cvc/verdandi/internal/verdandi"
)

func main() {
	jsonOutput := flag.Bool("json", false, "print JSON output")
	analyze := flag.Bool("analyze", false, "analyze without executing")
	status := flag.String("status", "", "lookup a run by runId")
	openOutput := flag.String("open-output", "", "list output files for a runId")
	dataDir := flag.String("data-dir", "", "runtime data directory")
	analyzer := flag.String("analyzer", "", "request analyzer backend: keyword, llm, or auto")
	llmEndpoint := flag.String("llm-endpoint", "", "LLM analyzer endpoint")
	llmModel := flag.String("llm-model", "", "LLM analyzer model")
	flag.Parse()

	tool := verdandi.NewTool(verdandi.Options{
		DataDir:  *dataDir,
		Analyzer: *analyzer,
		LLM: verdandi.LLMAnalyzerConfig{
			Endpoint: *llmEndpoint,
			Model:    *llmModel,
		},
	})

	var (
		result map[string]any
		err    error
	)

	switch {
	case *status != "":
		result, err = tool.GetStatus(*status)
	case *openOutput != "":
		result, err = tool.OpenOutput(*openOutput)
	default:
		request := strings.TrimSpace(strings.Join(flag.Args(), " "))
		if request == "" {
			fmt.Fprintln(os.Stderr, `usage: verdandi [--json] [--analyze] "자연어 작업 요청"`)
			os.Exit(1)
		}
		if *analyze {
			result, err = tool.Analyze(request)
		} else {
			result, err = tool.Run(request)
		}
	}

	if err != nil {
		printResult(map[string]any{"ok": false, "error": err.Error()}, true)
		os.Exit(1)
	}
	printResult(result, *jsonOutput)
}

func printResult(result map[string]any, asJSON bool) {
	if asJSON {
		encoded, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(encoded))
		return
	}

	if action, _ := result["action"].(string); action == "analyze" {
		fmt.Printf("intent: %v\n", result["intent"])
		fmt.Printf("confidence: %.0f%%\n", result["confidence"].(float64)*100)
		if plan, ok := result["plan"].(verdandi.Plan); ok {
			stages := make([]string, 0, len(plan.Stages))
			for _, stage := range plan.Stages {
				stages = append(stages, stage.Stage)
			}
			fmt.Printf("stages: %s\n", strings.Join(stages, " -> "))
		}
		return
	}

	fmt.Printf("status: %v\n", result["status"])
	if runID, ok := result["runId"].(string); ok {
		fmt.Printf("runId: %s\n", runID)
	}
	if outputDir, ok := result["outputDir"].(string); ok && outputDir != "" {
		fmt.Printf("output: %s\n", outputDir)
	}
}
