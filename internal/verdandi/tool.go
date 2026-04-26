package verdandi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Options struct {
	DataDir  string
	Analyzer string
	LLM      LLMAnalyzerConfig
}

type Tool struct {
	analyzer     Analyzer
	orchestrator Orchestrator
	store        Store
	agents       AgentRegistry
	events       EventStore
}

func NewTool(options Options) Tool {
	dataDir := options.DataDir
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	orchestrator := NewOrchestrator(dataDir)
	agents := NewAgentRegistry(filepath.Join(dataDir, "agents.json"))
	llmConfig := options.LLM
	if existingAgents, err := agents.List(); err == nil {
		llmConfig.ExistingAgents = existingAgents
	}
	return Tool{
		analyzer: NewAnalyzer(AnalyzerConfig{
			Mode:         options.Analyzer,
			Orchestrator: orchestrator,
			LLM:          llmConfig,
		}),
		orchestrator: orchestrator,
		store:        NewStoreForDataDir(dataDir),
		agents:       agents,
		events:       NewEventStoreForDataDir(dataDir),
	}
}

func NewToolWithAnalyzer(options Options, analyzer Analyzer) Tool {
	tool := NewTool(options)
	tool.analyzer = analyzer
	return tool
}

func (t Tool) Analyze(request string) (map[string]any, error) {
	if request == "" {
		return nil, fmt.Errorf("request 문자열이 필요합니다")
	}

	analysis, err := t.analyzeRequest(request)
	if err != nil {
		return nil, err
	}
	contract := AgentContract{
		Name:        agentName(analysis.Intent.Category),
		Description: "Natural-language Verdandi agent contract",
		Command:     "verdandi",
		Spec: AgentSpec{
			Role:         analysis.Intent.Category,
			Capabilities: capabilitiesFor(analysis.Intent.Category),
		},
		Metadata: map[string]any{
			"intent_analysis": analysis.Intent,
			"complexity":      analysis.Complexity,
			"analyzer":        analysis.Source,
		},
		Inputs: map[string]string{
			"request": request,
		},
	}

	result := map[string]any{
		"ok":         true,
		"action":     "analyze",
		"intent":     analysis.Intent.Category,
		"confidence": analysis.Intent.Confidence,
		"agent":      contract,
		"plan":       analysis.Plan,
		"analyzer":   analysis.Source,
	}
	if analysis.FallbackReason != "" {
		contract.Metadata["fallbackReason"] = analysis.FallbackReason
		result["fallbackReason"] = analysis.FallbackReason
	}
	return result, nil
}

func (t Tool) Run(request string, options ...map[string]any) (map[string]any, error) {
	runOptions := map[string]any{}
	if len(options) > 0 && options[0] != nil {
		runOptions = options[0]
	}
	analysis, err := t.analyzeRequest(request)
	if err != nil {
		return nil, err
	}
	plan, err := t.agents.ResolvePlan(analysis.Plan, runOptions)
	if err != nil {
		return nil, err
	}
	runID := createRunID()
	createdAt := time.Now().UTC()
	if err := t.events.Reset(runID); err != nil {
		return nil, err
	}
	if err := t.events.Append(runStartedEvent(runID, "running", request, createdAt)); err != nil {
		return nil, err
	}

	var eventErr error
	result, err := t.orchestrator.ExecutePlanWithObserver(plan, runOptions, func(phase StageLifecyclePhase, stage StageResult) {
		if eventErr != nil {
			return
		}
		switch phase {
		case StageLifecycleStarted:
			eventErr = t.events.AppendMany(stageStartedEvents(runID, stage))
		case StageLifecycleCompleted:
			eventErr = t.events.AppendMany(stageCompletedEvents(runID, stage))
		}
	})
	if eventErr != nil {
		return nil, eventErr
	}
	if err != nil {
		return nil, err
	}
	result.Analyzer = analysis.Source
	result.FallbackReason = analysis.FallbackReason
	if err := t.agents.RecordStageResults(result.Stages); err != nil {
		return nil, err
	}

	status := "success"
	if result.Summary.Failed > 0 {
		status = "error"
	}

	record := RunRecord{
		RunID:          runID,
		Status:         status,
		Request:        request,
		Analyzer:       result.Analyzer,
		FallbackReason: result.FallbackReason,
		OutputDir:      result.OutputDir,
		Summary:        result.Summary,
		Stages:         result.Stages,
		CreatedAt:      createdAt,
		CompletedAt:    result.CompletedAt,
	}
	if err := t.store.Save(record); err != nil {
		return nil, err
	}
	if err := t.events.Append(runCompletedEvent(record.RunID, record.Status, record.CompletedAt)); err != nil {
		return nil, err
	}

	response := map[string]any{
		"ok":        status == "success",
		"action":    "run",
		"runId":     record.RunID,
		"status":    record.Status,
		"request":   request,
		"analyzer":  record.Analyzer,
		"summary":   record.Summary,
		"outputDir": record.OutputDir,
	}
	if record.FallbackReason != "" {
		response["fallbackReason"] = record.FallbackReason
	}
	return response, nil
}

func (t Tool) analyzeRequest(request string) (AnalysisResult, error) {
	analyzer := t.analyzer
	if llm, ok := analyzer.(LLMAnalyzer); ok {
		if agents, err := t.agents.List(); err == nil {
			llm.config.ExistingAgents = agents
		}
		analyzer = llm
	}
	return analyzer.Analyze(request)
}

func (t Tool) Orchestrate(request string, options map[string]any) (map[string]any, error) {
	result, err := t.Run(request, options)
	if err != nil {
		return nil, err
	}
	result["action"] = "orchestrate"
	return result, nil
}

func (t Tool) GetStatus(runID string) (map[string]any, error) {
	record, err := t.store.Find(runID)
	if err != nil {
		return nil, err
	}
	response := map[string]any{
		"ok":           true,
		"action":       "status",
		"runId":        record.RunID,
		"status":       record.Status,
		"request":      record.Request,
		"analyzer":     record.Analyzer,
		"outputDir":    record.OutputDir,
		"summary":      record.Summary,
		"stages":       record.Stages,
		"created_at":   record.CreatedAt,
		"completed_at": record.CompletedAt,
	}
	if record.FallbackReason != "" {
		response["fallbackReason"] = record.FallbackReason
	}
	return response, nil
}

func (t Tool) ListAgents() (map[string]any, error) {
	agents, err := t.agents.List()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":      true,
		"action":  "list_agents",
		"count":   len(agents),
		"agents":  agents,
		"options": agentPolicyOptions(),
	}, nil
}

func (t Tool) OpenOutput(runID string) (map[string]any, error) {
	record, err := t.store.Find(runID)
	if err != nil {
		return nil, err
	}
	if record.OutputDir == "" {
		return nil, fmt.Errorf("출력 디렉토리가 없습니다")
	}

	entries, err := os.ReadDir(record.OutputDir)
	if err != nil {
		return nil, err
	}

	files := []FileInfo{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, FileInfo{
			Name:        entry.Name(),
			Path:        filepath.Join(record.OutputDir, entry.Name()),
			Size:        info.Size(),
			IsDirectory: entry.IsDir(),
		})
	}

	return map[string]any{
		"ok":        true,
		"action":    "open_output",
		"runId":     runID,
		"outputDir": record.OutputDir,
		"files":     files,
	}, nil
}

func (t Tool) Handle(action string, params map[string]any) (map[string]any, error) {
	switch action {
	case "run":
		return t.Run(requiredString(params, "request"), optionalObject(params, "options"))
	case "analyze":
		return t.Analyze(requiredString(params, "request"))
	case "orchestrate":
		return t.Orchestrate(requiredString(params, "request"), optionalObject(params, "options"))
	case "status", "get_status":
		return t.GetStatus(requiredString(params, "runId"))
	case "list_agents":
		return t.ListAgents()
	case "open_output":
		return t.OpenOutput(requiredString(params, "runId"))
	default:
		return nil, fmt.Errorf("알 수 없는 Verdandi action: %s", action)
	}
}

func DefaultDataDir() string {
	if value := os.Getenv("VERDANDI_DATA_DIR"); value != "" {
		return value
	}
	return ".verdandi"
}

func requiredString(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	value, _ := params[key].(string)
	return value
}

func optionalObject(params map[string]any, key string) map[string]any {
	if params == nil {
		return nil
	}
	value, ok := params[key].(map[string]any)
	if !ok {
		return nil
	}
	return value
}

func createRunID() string {
	return fmt.Sprintf("run_%d", time.Now().UTC().UnixNano())
}

func agentName(intent string) string {
	switch intent {
	case IntentOrchestrator:
		return "VerdandiOrchestrator"
	case IntentPlanner:
		return "VerdandiPlanner"
	case IntentDocumenter:
		return "VerdandiDocumenter"
	case IntentResearcher:
		return "VerdandiResearcher"
	default:
		return "VerdandiAgent"
	}
}

func capabilitiesFor(intent string) []string {
	common := []string{"natural-language-request", "context-aware-execution"}
	switch intent {
	case IntentOrchestrator:
		return append(common, "workflow-planning", "agent-coordination")
	case IntentPlanner:
		return append(common, "requirements-analysis", "planning")
	case IntentDocumenter:
		return append(common, "documentation")
	case IntentCodeWriter:
		return append(common, "code-generation", "validation")
	default:
		return common
	}
}

func toJSON(value any) string {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}
