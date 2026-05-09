package verdandi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	assets       AssetRegistry
	events       EventStore
	dataDir      string
}

type ProgressEvent struct {
	Progress int
	Total    int
	Message  string
}

type ProgressReporter func(ProgressEvent)

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
		assets:       NewAssetRegistry(dataDir),
		events:       NewEventStoreForDataDir(dataDir),
		dataDir:      dataDir,
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
	return t.RunContext(context.Background(), request, nil, options...)
}

func (t Tool) RunWithProgress(request string, reporter ProgressReporter, options ...map[string]any) (map[string]any, error) {
	return t.RunContext(context.Background(), request, reporter, options...)
}

func (t Tool) RunContext(ctx context.Context, request string, reporter ProgressReporter, options ...map[string]any) (map[string]any, error) {
	reportProgress(reporter, 0, 1, "prepare_workflow started")
	result, err := t.PrepareWorkflow(request, options...)
	if err != nil {
		return nil, err
	}
	result["action"] = "run"
	reportProgress(reporter, 1, 1, "prepare_workflow completed")
	return result, nil
}

func (t Tool) RunPlan(request string, stages []StageDef, options ...map[string]any) (map[string]any, error) {
	return t.RunPlanContext(context.Background(), request, stages, nil, options...)
}

func (t Tool) RunPlanWithProgress(request string, stages []StageDef, reporter ProgressReporter, options ...map[string]any) (map[string]any, error) {
	return t.RunPlanContext(context.Background(), request, stages, reporter, options...)
}

func (t Tool) RunPlanContext(ctx context.Context, request string, stages []StageDef, reporter ProgressReporter, options ...map[string]any) (map[string]any, error) {
	if strings.TrimSpace(request) == "" {
		return nil, fmt.Errorf("request 문자열이 필요합니다")
	}
	runOptions := map[string]any{}
	if len(options) > 0 && options[0] != nil {
		runOptions = options[0]
	}
	plan, err := t.normalizeClientPlan(request, stages)
	if err != nil {
		return nil, err
	}
	plan, err = t.agents.ResolvePlan(plan, runOptions)
	if err != nil {
		return nil, err
	}
	return t.executePlan(ctx, request, plan, runOptions, AnalyzerClient, "", "run_plan", reporter)
}

func (t Tool) ValidatePlan(request string, stages []StageDef) (map[string]any, error) {
	if strings.TrimSpace(request) == "" {
		return nil, fmt.Errorf("request 문자열이 필요합니다")
	}
	plan, err := t.normalizeClientPlan(request, stages)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":         true,
		"action":     "validate_plan",
		"request":    request,
		"stageCount": plan.StageCount,
		"plan":       plan,
		"analyzer":   AnalyzerClient,
	}, nil
}

func (t Tool) PrepareWorkflow(request string, options ...map[string]any) (map[string]any, error) {
	if strings.TrimSpace(request) == "" {
		return nil, fmt.Errorf("request 문자열이 필요합니다")
	}
	runOptions := map[string]any{}
	if len(options) > 0 && options[0] != nil {
		runOptions = options[0]
	}

	analysis, err := t.analyzeRequest(request)
	if err != nil {
		return nil, err
	}

	runID := createRunID()
	createdAt := time.Now().UTC()
	pkg := BuildWorkflowPackageFromPlan(runID, request, analysis.Plan)
	existingAgents, err := t.assets.ListAgents()
	if err != nil {
		return nil, err
	}
	for index, agent := range pkg.Agents {
		originalID := agent.ID
		selected, err := t.selectPreparedAgentAsset(existingAgents, agent, runOptions)
		if err != nil {
			return nil, err
		}
		if !containsAgentAssetID(existingAgents, selected.ID) {
			existingAgents = append(existingAgents, selected)
		}
		pkg.Agents[index] = selected
		replaceWorkflowAgentID(&pkg, originalID, selected.ID)
	}
	for _, skill := range pkg.Skills {
		if err := t.assets.UpsertSkill(skill); err != nil {
			return nil, err
		}
	}

	written, err := WriteWorkflowPackage(t.dataDir, pkg)
	if err != nil {
		return nil, err
	}
	pkg = written.Package
	completedAt := time.Now().UTC()
	summary := Summary{
		TotalStages: len(pkg.Tasks),
		Success:     0,
		Failed:      0,
		OutputDir:   written.OutputDir,
		Files:       written.Files,
	}
	record := RunRecord{
		RunID:          runID,
		Status:         "prepared",
		Request:        request,
		Analyzer:       analysis.Source,
		FallbackReason: analysis.FallbackReason,
		OutputDir:      written.OutputDir,
		Summary:        summary,
		Stages:         preparedStageResults(pkg.Tasks, createdAt, completedAt),
		CreatedAt:      createdAt,
		CompletedAt:    completedAt,
	}
	if err := t.store.Save(record); err != nil {
		return nil, err
	}
	if err := t.events.SaveRun(record); err != nil {
		return nil, err
	}

	result := map[string]any{
		"ok":        true,
		"action":    "prepare_workflow",
		"runId":     record.RunID,
		"status":    record.Status,
		"request":   request,
		"analyzer":  record.Analyzer,
		"outputDir": record.OutputDir,
		"workflow":  pkg,
		"summary":   summary,
	}
	if record.FallbackReason != "" {
		result["fallbackReason"] = record.FallbackReason
	}
	return result, nil
}

func (t Tool) normalizeClientPlan(request string, stages []StageDef) (Plan, error) {
	if len(stages) == 0 {
		return Plan{}, fmt.Errorf("stages 배열이 필요합니다")
	}
	return t.orchestrator.NormalizePlan(request, stages)
}

func (t Tool) executePlan(ctx context.Context, request string, plan Plan, runOptions map[string]any, analyzer string, fallbackReason string, action string, reporter ProgressReporter) (map[string]any, error) {
	runID := createRunID()
	createdAt := time.Now().UTC()
	if err := t.events.Reset(runID); err != nil {
		return nil, err
	}
	if err := t.events.Append(runStartedEvent(runID, "running", request, createdAt)); err != nil {
		return nil, err
	}
	totalStages := len(plan.Stages)
	var eventErr error
	completedStages := 0
	result, err := t.orchestrator.ExecutePlanWithObserverContext(ctx, plan, runOptions, func(phase StageLifecyclePhase, stage StageResult) {
		if eventErr != nil {
			return
		}
		switch phase {
		case StageLifecycleStarted:
			reportProgress(reporter, completedStages, totalStages, fmt.Sprintf("%s started", stage.Stage))
			eventErr = t.events.AppendMany(stageStartedEvents(runID, stage))
		case StageLifecycleCompleted:
			completedStages++
			reportProgress(reporter, completedStages, totalStages, fmt.Sprintf("%s completed", stage.Stage))
			eventErr = t.events.AppendMany(stageCompletedEvents(runID, stage))
		}
	})
	if eventErr != nil {
		return nil, eventErr
	}
	if err != nil {
		return nil, err
	}
	result.Analyzer = analyzer
	result.FallbackReason = fallbackReason
	if err := t.agents.RecordStageResults(result.Stages); err != nil {
		return nil, err
	}
	result.Stages = t.stagesWithCurrentAgentMetrics(result.Stages)
	for _, stage := range result.Stages {
		if err := t.events.AppendMany(metricsUpdatedEvents(runID, stage)); err != nil {
			return nil, err
		}
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
		"action":    action,
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

func reportProgress(reporter ProgressReporter, progress int, total int, message string) {
	if reporter == nil {
		return
	}
	reporter(ProgressEvent{Progress: progress, Total: total, Message: message})
}

func (t Tool) stagesWithCurrentAgentMetrics(stages []StageResult) []StageResult {
	agents, err := t.agents.List()
	if err != nil {
		return stages
	}
	agentsByName := map[string]AgentContract{}
	for _, agent := range agents {
		agentsByName[agent.Name] = agent
	}
	updated := make([]StageResult, len(stages))
	copy(updated, stages)
	for index := range updated {
		if updated[index].Agent == nil {
			continue
		}
		agent, ok := agentsByName[updated[index].Agent.Name]
		if !ok {
			continue
		}
		updated[index].Agent = &agent
	}
	return updated
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
	result, err := t.RunWithProgress(request, nil, options)
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

func (t Tool) ListSkills() (map[string]any, error) {
	skills, err := t.assets.ListSkills()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":     true,
		"action": "list_skills",
		"count":  len(skills),
		"skills": skills,
	}, nil
}

func (t Tool) ListAssets() (map[string]any, error) {
	agents, err := t.assets.ListAgents()
	if err != nil {
		return nil, err
	}
	skills, err := t.assets.ListSkills()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":      true,
		"action":  "list_assets",
		"agents":  agents,
		"skills":  skills,
		"counts":  map[string]int{"agents": len(agents), "skills": len(skills)},
		"options": agentPolicyOptions(),
	}, nil
}

func (t Tool) RecommendAssets(request string) (map[string]any, error) {
	agents, err := t.assets.ListAgents()
	if err != nil {
		return nil, err
	}
	skills, err := t.assets.ListSkills()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":      true,
		"action":  "recommend_assets",
		"request": request,
		"agents":  activeAgentAssets(agents),
		"skills":  activeSkillAssets(skills),
	}, nil
}

func (t Tool) RecordOutcome(args map[string]any) (map[string]any, error) {
	assetID := requiredString(args, "assetId")
	if strings.TrimSpace(assetID) == "" {
		return nil, fmt.Errorf("assetId 문자열이 필요합니다")
	}
	kind := requiredString(args, "kind")
	if strings.TrimSpace(kind) == "" {
		return nil, fmt.Errorf("kind 문자열이 필요합니다")
	}
	if kind != AssetKindAgent && kind != AssetKindSkill {
		return nil, fmt.Errorf("kind must be %q or %q", AssetKindAgent, AssetKindSkill)
	}
	status := requiredString(args, "status")
	if strings.TrimSpace(status) == "" {
		return nil, fmt.Errorf("status 문자열이 필요합니다")
	}
	if status != "success" && status != "error" {
		return nil, fmt.Errorf("status must be %q or %q", "success", "error")
	}

	outcome := AssetOutcome{
		AssetID:     assetID,
		Kind:        kind,
		Status:      status,
		CompletedAt: time.Now().UTC(),
	}
	if value, ok := args["runId"].(string); ok {
		outcome.RunID = value
	}
	if value, ok := args["error"].(string); ok {
		outcome.Error = value
	}
	if value, ok := args["lesson"].(string); ok {
		outcome.Lesson = value
	}
	if err := t.assets.RecordOutcome(outcome); err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":      true,
		"action":  "record_outcome",
		"assetId": assetID,
		"kind":    kind,
		"status":  status,
	}, nil
}

func (t Tool) GetWorkflow(runID string) (map[string]any, error) {
	record, err := t.workflowRunRecord(runID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(record.OutputDir, "workflow.json"))
	if err != nil {
		return nil, err
	}
	var workflow any
	if err := json.Unmarshal(data, &workflow); err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":       true,
		"action":   "get_workflow",
		"runId":    record.RunID,
		"workflow": workflow,
	}, nil
}

func (t Tool) GetWorkflowHandoff(runID string) (map[string]any, error) {
	record, err := t.workflowRunRecord(runID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(record.OutputDir, "handoff.md"))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":      true,
		"action":  "get_workflow_handoff",
		"runId":   record.RunID,
		"handoff": string(data),
	}, nil
}

func (t Tool) workflowRunRecord(runID string) (RunRecord, error) {
	if strings.TrimSpace(runID) == "" {
		return RunRecord{}, fmt.Errorf("runId 문자열이 필요합니다")
	}
	record, err := t.store.Find(runID)
	if err != nil {
		return RunRecord{}, err
	}
	if strings.TrimSpace(record.OutputDir) == "" {
		return RunRecord{}, fmt.Errorf("workflow 출력 디렉토리가 없습니다")
	}
	return record, nil
}

func (t Tool) ListRuns() (map[string]any, error) {
	runs, err := t.store.List()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":     true,
		"action": "list_runs",
		"count":  len(runs),
		"runs":   runs,
	}, nil
}

func (t Tool) ListEvents(runID string) (map[string]any, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("runId 문자열이 필요합니다")
	}
	events, err := t.events.List(runID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":     true,
		"action": "list_events",
		"runId":  runID,
		"count":  len(events),
		"events": events,
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
	return t.HandleWithProgress(action, params, nil)
}

func (t Tool) HandleWithProgress(action string, params map[string]any, reporter ProgressReporter) (map[string]any, error) {
	return t.HandleContext(context.Background(), action, params, reporter)
}

func (t Tool) HandleContext(ctx context.Context, action string, params map[string]any, reporter ProgressReporter) (map[string]any, error) {
	switch action {
	case "run":
		return t.RunContext(ctx, requiredString(params, "request"), reporter, optionalObject(params, "options"))
	case "analyze":
		return t.Analyze(requiredString(params, "request"))
	case "run_plan":
		stages, err := requiredStages(params)
		if err != nil {
			return nil, err
		}
		return t.RunPlanContext(ctx, requiredString(params, "request"), stages, reporter, optionalObject(params, "options"))
	case "validate_plan":
		stages, err := requiredStages(params)
		if err != nil {
			return nil, err
		}
		return t.ValidatePlan(requiredString(params, "request"), stages)
	case "prepare_workflow":
		return t.PrepareWorkflow(requiredString(params, "request"), optionalObject(params, "options"))
	case "recommend_assets":
		return t.RecommendAssets(requiredString(params, "request"))
	case "record_outcome":
		return t.RecordOutcome(params)
	case "orchestrate":
		return t.Orchestrate(requiredString(params, "request"), optionalObject(params, "options"))
	case "status", "get_status":
		return t.GetStatus(requiredString(params, "runId"))
	case "list_runs":
		return t.ListRuns()
	case "list_events":
		return t.ListEvents(requiredString(params, "runId"))
	case "list_agents":
		return t.ListAgents()
	case "list_assets":
		return t.ListAssets()
	case "list_skills":
		return t.ListSkills()
	case "get_workflow":
		return t.GetWorkflow(requiredString(params, "runId"))
	case "get_workflow_handoff":
		return t.GetWorkflowHandoff(requiredString(params, "runId"))
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

func findReusableAgentAsset(existing []AgentAsset, candidate AgentAsset) (AgentAsset, bool) {
	for _, agent := range existing {
		if !strings.EqualFold(agent.Name, candidate.Name) {
			continue
		}
		if agent.Status != AssetStatusActive {
			continue
		}
		return agent, true
	}
	return AgentAsset{}, false
}

func (t Tool) selectPreparedAgentAsset(existing []AgentAsset, candidate AgentAsset, options map[string]any) (AgentAsset, error) {
	active, found := findReusableAgentAsset(existing, candidate)
	policy, hasPolicy := explicitAgentPolicy(options)
	if !hasPolicy {
		if found {
			return active, nil
		}
		return t.assets.SaveAgentVersion(candidate, "")
	}
	switch policy {
	case AgentPolicyReuseEnhance:
		if found {
			return active, nil
		}
		return t.assets.SaveAgentVersion(candidate, "")
	case AgentPolicyRewrite:
		if found {
			return t.assets.SaveAgentVersion(candidate, active.ID)
		}
		return t.assets.SaveAgentVersion(candidate, "")
	case AgentPolicySeparate:
		if found {
			candidate.Name = uniqueSeparateAssetName(candidate.Name, existing)
			candidate.ID = ""
		}
		return t.assets.SaveAgentVersion(candidate, "")
	default:
		return AgentAsset{}, fmt.Errorf("unknown agent policy: %s", policy)
	}
}

func containsAgentAssetID(agents []AgentAsset, id string) bool {
	for _, agent := range agents {
		if agent.ID == id {
			return true
		}
	}
	return false
}

func uniqueSeparateAssetName(base string, existing []AgentAsset) string {
	if strings.TrimSpace(base) == "" {
		base = "SeparateAgent"
	}
	names := map[string]bool{}
	for _, agent := range existing {
		names[strings.ToLower(agent.Name)] = true
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%sSeparate%d", base, index)
		if !names[strings.ToLower(candidate)] {
			return candidate
		}
	}
}

func replaceWorkflowAgentID(pkg *WorkflowPackage, oldID string, newID string) {
	if oldID == newID {
		return
	}
	for taskIndex := range pkg.Tasks {
		if pkg.Tasks[taskIndex].AgentID == oldID {
			pkg.Tasks[taskIndex].AgentID = newID
		}
	}
	for skillIndex := range pkg.Skills {
		for usedByIndex, usedByAgent := range pkg.Skills[skillIndex].UsedByAgents {
			if usedByAgent == oldID {
				pkg.Skills[skillIndex].UsedByAgents[usedByIndex] = newID
			}
		}
	}
}

func activeAgentAssets(agents []AgentAsset) []AgentAsset {
	filtered := make([]AgentAsset, 0, len(agents))
	for _, agent := range agents {
		if agent.Status != AssetStatusActive {
			continue
		}
		filtered = append(filtered, agent)
	}
	return filtered
}

func activeSkillAssets(skills []SkillAsset) []SkillAsset {
	filtered := make([]SkillAsset, 0, len(skills))
	for _, skill := range skills {
		if skill.Status != AssetStatusActive {
			continue
		}
		filtered = append(filtered, skill)
	}
	return filtered
}

func preparedStageResults(tasks []WorkflowTask, started time.Time, ended time.Time) []StageResult {
	stages := make([]StageResult, 0, len(tasks))
	for _, task := range tasks {
		stages = append(stages, StageResult{
			Stage:  task.ID,
			Status: "prepared",
			Result: &StageOutput{
				Type:    "workflow-task",
				Status:  "prepared",
				Message: task.Description,
			},
			Started: started,
			Ended:   ended,
		})
	}
	return stages
}

func requiredString(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	value, _ := params[key].(string)
	return value
}

func requiredStages(params map[string]any) ([]StageDef, error) {
	if params == nil {
		return nil, fmt.Errorf("stages 배열이 필요합니다")
	}
	raw, ok := params["stages"]
	if !ok {
		return nil, fmt.Errorf("stages 배열이 필요합니다")
	}
	if stages, ok := raw.([]StageDef); ok {
		if len(stages) == 0 {
			return nil, fmt.Errorf("stages 배열이 필요합니다")
		}
		return stages, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var stages []StageDef
	if err := json.Unmarshal(data, &stages); err != nil {
		return nil, err
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("stages 배열이 필요합니다")
	}
	return stages, nil
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
