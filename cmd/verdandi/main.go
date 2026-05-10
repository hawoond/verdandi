package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/genie-cvc/verdandi/internal/upgrade"
	"github.com/genie-cvc/verdandi/internal/verdandi"
	"github.com/genie-cvc/verdandi/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "upgrade" {
		os.Exit(runUpgrade(os.Args[2:], os.Stdout, os.Stderr))
	}

	jsonOutput := flag.Bool("json", false, "print JSON output")
	analyze := flag.Bool("analyze", false, "analyze without executing")
	status := flag.String("status", "", "lookup a run by runId")
	openOutput := flag.String("open-output", "", "list output files for a runId")
	dataDir := flag.String("data-dir", "", "runtime data directory")
	analyzer := flag.String("analyzer", "", "request analyzer backend: keyword, llm, or auto")
	llmEndpoint := flag.String("llm-endpoint", "", "LLM analyzer endpoint")
	llmModel := flag.String("llm-model", "", "LLM analyzer model")
	showVersion := flag.Bool("version", false, "print version metadata")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String())
		return
	}

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

func runUpgrade(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	flags.SetOutput(stderr)
	targetVersion := flags.String("version", "", "release version to install, for example 0.0.2; defaults to latest")
	installDir := flags.String("install-dir", "", "directory to install Verdandi binaries; defaults to current executable directory")
	dryRun := flags.Bool("dry-run", false, "show the selected release archive without installing")
	force := flags.Bool("force", false, "install even when the selected version matches the current binary version")
	repository := flags.String("repo", "hawoond/verdandi", "GitHub repository in owner/name form")
	apiURL := flags.String("api-url", "", "GitHub API base URL")
	jsonOutput := flags.Bool("json", false, "print JSON output")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	result, err := upgrade.Run(upgrade.Options{
		Version:    *targetVersion,
		InstallDir: *installDir,
		Repository: *repository,
		APIBaseURL: *apiURL,
		DryRun:     *dryRun,
		Force:      *force,
	})
	if err != nil {
		fmt.Fprintf(stderr, "upgrade failed: %v\n", err)
		return 1
	}
	if *jsonOutput {
		payload := map[string]any{
			"action":          "upgrade",
			"version":         result.Version,
			"tagName":         result.TagName,
			"archiveName":     result.ArchiveName,
			"installDir":      result.InstallDir,
			"dryRun":          result.DryRun,
			"alreadyUpToDate": result.AlreadyUpToDate,
		}
		encoded, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Fprintln(stdout, string(encoded))
		return 0
	}
	if result.AlreadyUpToDate {
		fmt.Fprintf(stdout, "Verdandi is already up to date: %s\n", result.TagName)
		return 0
	}
	if result.DryRun {
		fmt.Fprintf(stdout, "upgrade dry-run: %s\narchive: %s\ninstallDir: %s\n", result.TagName, result.ArchiveName, result.InstallDir)
		return 0
	}
	fmt.Fprintf(stdout, "upgraded Verdandi to %s\narchive: %s\ninstallDir: %s\n", result.TagName, result.ArchiveName, result.InstallDir)
	return 0
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
