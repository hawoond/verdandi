package verdandi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type staticAnalyzer struct {
	result AnalysisResult
	err    error
}

func (a staticAnalyzer) Analyze(request string) (AnalysisResult, error) {
	if a.err != nil {
		return AnalysisResult{}, a.err
	}
	a.result.Text = request
	return a.result, nil
}

func TestToolRunAcceptsNaturalLanguageRequest(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	result, err := tool.Handle("run", map[string]any{
		"request": "계산기 앱을 기획하고 구현하고 테스트해줘",
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if result["ok"] != true {
		t.Fatalf("expected ok result, got %#v", result)
	}
	if result["action"] != "run" {
		t.Fatalf("expected run action, got %#v", result["action"])
	}
	if result["runId"] == "" {
		t.Fatalf("expected runId, got %#v", result)
	}
	if result["request"] != "계산기 앱을 기획하고 구현하고 테스트해줘" {
		t.Fatalf("request was not echoed: %#v", result)
	}
}

func TestToolAnalyzeReturnsExecutionPlan(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	result, err := tool.Analyze("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if result["action"] != "analyze" {
		t.Fatalf("expected analyze action, got %#v", result["action"])
	}

	plan, ok := result["plan"].(Plan)
	if !ok {
		t.Fatalf("expected typed Plan in analyze result, got %#v", result["plan"])
	}
	if plan.StageCount != 4 {
		t.Fatalf("expected 4 stages, got %#v", plan)
	}

	got := []string{}
	for _, stage := range plan.Stages {
		got = append(got, stage.Stage)
	}
	want := []string{"planner", "code-writer", "tester", "documenter"}
	if !equalStrings(got, want) {
		t.Fatalf("stages mismatch: got %#v want %#v", got, want)
	}
}

func TestToolUsesConfiguredAnalyzer(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{
		DataDir:  dataDir,
		Analyzer: AnalyzerKeyword,
	})

	result, err := tool.Analyze("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if result["intent"] != IntentPlanner {
		t.Fatalf("expected planner intent, got %#v", result["intent"])
	}
	plan := result["plan"].(Plan)
	if plan.StageCount != 4 {
		t.Fatalf("expected 4 stages, got %#v", plan)
	}
}

func TestToolStoresStatusAndListsOutput(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	result, err := tool.Run("문서 작성")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	runID := result["runId"].(string)
	status, err := tool.GetStatus(runID)
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	if status["status"] != "success" {
		t.Fatalf("expected success status, got %#v", status["status"])
	}

	opened, err := tool.OpenOutput(runID)
	if err != nil {
		t.Fatalf("open output failed: %v", err)
	}
	files := opened["files"].([]FileInfo)
	if len(files) == 0 {
		t.Fatalf("expected output files")
	}
	for _, file := range files {
		if filepath.IsAbs(file.Name) {
			t.Fatalf("file name should not be absolute: %#v", file)
		}
	}
}

func TestToolStatusIncludesTypedStageResults(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewTool(Options{DataDir: dataDir})

	result, err := tool.Run("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	status, err := tool.GetStatus(result["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	stages := status["stages"].([]StageResult)
	if len(stages) != 4 {
		t.Fatalf("expected 4 stages, got %#v", stages)
	}
	for _, stage := range stages {
		if stage.Result == nil {
			t.Fatalf("expected result for stage %s", stage.Stage)
		}
	}
}

func TestToolRunUsesAnalyzerPlan(t *testing.T) {
	dataDir := t.TempDir()
	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan("문서만 작성", []StageDef{
		{Stage: "documenter", Keyword: "llm"},
	})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{
		result: AnalysisResult{
			Intent:     IntentResult{Category: IntentDocumenter, Confidence: 0.9},
			Complexity: ComplexityResult{Level: "LOW", Score: 1},
			Plan:       plan,
			Source:     AnalyzerLLM,
		},
	})

	result, err := tool.Run("문서만 작성")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if result["analyzer"] != AnalyzerLLM {
		t.Fatalf("expected llm analyzer, got %#v", result["analyzer"])
	}
	summary := result["summary"].(Summary)
	if summary.TotalStages != 1 {
		t.Fatalf("expected analyzer plan to drive one stage, got %#v", summary)
	}

	runID := result["runId"].(string)
	status, err := tool.GetStatus(runID)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stages := status["stages"].([]StageResult)
	if len(stages) != 1 || stages[0].Stage != "documenter" {
		t.Fatalf("expected documenter-only run, got %#v", stages)
	}
	if status["analyzer"] != AnalyzerLLM {
		t.Fatalf("expected analyzer in status, got %#v", status["analyzer"])
	}
}

func TestToolPropagatesFallbackReason(t *testing.T) {
	dataDir := t.TempDir()
	orchestrator := NewOrchestrator(dataDir)
	plan := orchestrator.ParseRequest("기획 구현 테스트 문서화")
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{
		result: AnalysisResult{
			Intent:         IntentResult{Category: IntentPlanner, Confidence: 0.2},
			Complexity:     ComplexityResult{Level: "LOW", Score: 0},
			Plan:           plan,
			Source:         AnalyzerKeyword,
			FallbackReason: "llm analyzer returned invalid stage",
		},
	})

	analyzed, err := tool.Analyze("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if analyzed["fallbackReason"] != "llm analyzer returned invalid stage" {
		t.Fatalf("expected analyze fallback reason, got %#v", analyzed["fallbackReason"])
	}

	run, err := tool.Run("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if run["fallbackReason"] != "llm analyzer returned invalid stage" {
		t.Fatalf("expected run fallback reason, got %#v", run["fallbackReason"])
	}

	status, err := tool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status["fallbackReason"] != "llm analyzer returned invalid stage" {
		t.Fatalf("expected status fallback reason, got %#v", status["fallbackReason"])
	}
}

func TestToolAnalyzeAndRunExposeDynamicAgents(t *testing.T) {
	dataDir := t.TempDir()
	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan("접근성 좋은 계산기 앱 구현", []StageDef{
		{
			Stage:   "code-writer",
			Keyword: "llm",
			Agent: &AgentContract{
				Name:        "AccessibilityFocusedFrontendAgent",
				Description: "Builds UI code with accessibility checks.",
				Spec: AgentSpec{
					Role:         "frontend accessibility engineer",
					Capabilities: []string{"ui-implementation", "accessibility", "validation"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{
		result: AnalysisResult{
			Intent:     IntentResult{Category: IntentCodeWriter, Confidence: 0.91},
			Complexity: ComplexityResult{Level: "MEDIUM", Score: 4},
			Plan:       plan,
			Source:     AnalyzerLLM,
		},
	})

	analyzed, err := tool.Analyze("접근성 좋은 계산기 앱 구현")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	analyzedPlan := analyzed["plan"].(Plan)
	if analyzedPlan.Stages[0].Agent == nil || analyzedPlan.Stages[0].Agent.Name != "AccessibilityFocusedFrontendAgent" {
		t.Fatalf("expected dynamic agent in analyze plan, got %#v", analyzedPlan.Stages)
	}

	run, err := tool.Run("접근성 좋은 계산기 앱 구현")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	status, err := tool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stages := status["stages"].([]StageResult)
	if stages[0].Stage != "code-writer" || stages[0].Result == nil {
		t.Fatalf("expected code writer execution, got %#v", stages)
	}
	if stages[0].Agent == nil || stages[0].Agent.Name != "AccessibilityFocusedFrontendAgent" {
		t.Fatalf("expected selected dynamic agent in stage result, got %#v", stages[0].Agent)
	}
}

func TestToolRunReusesAndEnhancesSimilarAgentByDefault(t *testing.T) {
	dataDir := t.TempDir()
	firstTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "기존 접근성 UI 에이전트 생성", AgentContract{
		Name:        "ExistingAccessibilityAgent",
		Description: "Builds accessible interfaces.",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility", "semantic-html"},
		},
	}))
	if _, err := firstTool.Run("기존 접근성 UI 에이전트 생성"); err != nil {
		t.Fatalf("initial run failed: %v", err)
	}

	secondTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "접근성 좋은 대시보드 구현", AgentContract{
		Name:        "DashboardAccessibilityAgent",
		Description: "Builds accessible dashboards.",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility", "dashboard-ui"},
		},
	}))
	run, err := secondTool.Run("접근성 좋은 대시보드 구현")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	status, err := secondTool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stage := status["stages"].([]StageResult)[0]
	if stage.Agent == nil || stage.Agent.Name != "ExistingAccessibilityAgent" {
		t.Fatalf("expected similar agent to be reused, got %#v", stage.Agent)
	}
	if stage.AgentDecision == nil || stage.AgentDecision.Action != AgentPolicyReuseEnhance {
		t.Fatalf("expected reuse-enhance decision, got %#v", stage.AgentDecision)
	}
	if !containsString(stage.Agent.Spec.Capabilities, "dashboard-ui") {
		t.Fatalf("expected reused agent to gain new capability, got %#v", stage.Agent.Spec.Capabilities)
	}
}

func TestToolRunCanRewriteSimilarAgent(t *testing.T) {
	dataDir := t.TempDir()
	firstTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "기존 문서 에이전트 생성", AgentContract{
		Name:        "LegacyDocumentationAgent",
		Description: "Writes README files.",
		Spec: AgentSpec{
			Role:         "documentation engineer",
			Capabilities: []string{"documentation", "readme"},
		},
	}))
	if _, err := firstTool.Run("기존 문서 에이전트 생성"); err != nil {
		t.Fatalf("initial run failed: %v", err)
	}

	rewriteTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "문서 에이전트를 새 기준으로 재작성", AgentContract{
		Name:        "ModernDocumentationAgent",
		Description: "Writes product guides.",
		Spec: AgentSpec{
			Role:         "documentation engineer",
			Capabilities: []string{"documentation", "product-guide"},
		},
	}))
	run, err := rewriteTool.Run("문서 에이전트를 새 기준으로 재작성", map[string]any{"agentPolicy": AgentPolicyRewrite})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	status, err := rewriteTool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stage := status["stages"].([]StageResult)[0]
	if stage.Agent == nil || stage.Agent.Name != "ModernDocumentationAgent" {
		t.Fatalf("expected rewritten agent candidate to be selected, got %#v", stage.Agent)
	}
	if stage.AgentDecision == nil || stage.AgentDecision.Action != AgentPolicyRewrite {
		t.Fatalf("expected rewrite decision, got %#v", stage.AgentDecision)
	}
	if stage.AgentDecision.ExistingAgentName != "LegacyDocumentationAgent" {
		t.Fatalf("expected decision to identify rewritten agent, got %#v", stage.AgentDecision)
	}
}

func TestToolRunCanSeparateSimilarAgent(t *testing.T) {
	dataDir := t.TempDir()
	firstTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "기존 분석 에이전트 생성", AgentContract{
		Name:        "GeneralAnalysisAgent",
		Description: "Analyzes product metrics.",
		Spec: AgentSpec{
			Role:         "data analyst",
			Capabilities: []string{"data-analysis", "reporting"},
		},
	}))
	if _, err := firstTool.Run("기존 분석 에이전트 생성"); err != nil {
		t.Fatalf("initial run failed: %v", err)
	}

	separateTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "별도 실험 분석 에이전트 생성", AgentContract{
		Name:        "ExperimentAnalysisAgent",
		Description: "Analyzes experiments separately.",
		Spec: AgentSpec{
			Role:         "data analyst",
			Capabilities: []string{"data-analysis", "experimentation"},
		},
	}))
	run, err := separateTool.Run("별도 실험 분석 에이전트 생성", map[string]any{"agentPolicy": AgentPolicySeparate})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	status, err := separateTool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stage := status["stages"].([]StageResult)[0]
	if stage.Agent == nil || stage.Agent.Name != "ExperimentAnalysisAgent" {
		t.Fatalf("expected candidate to remain separate, got %#v", stage.Agent)
	}
	if stage.AgentDecision == nil || stage.AgentDecision.Action != AgentPolicySeparate {
		t.Fatalf("expected separate decision, got %#v", stage.AgentDecision)
	}
	if stage.AgentDecision.ExistingAgentName != "GeneralAnalysisAgent" {
		t.Fatalf("expected decision to identify similar existing agent, got %#v", stage.AgentDecision)
	}
}

func TestToolRunHonorsAnalyzerAgentLifecycleDecision(t *testing.T) {
	dataDir := t.TempDir()
	firstTool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "기존 접근성 에이전트 생성", AgentContract{
		Name:        "LegacyAccessibilityAgent",
		Description: "Builds basic accessible interfaces.",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility", "semantic-html"},
		},
	}))
	if _, err := firstTool.Run("기존 접근성 에이전트 생성"); err != nil {
		t.Fatalf("initial run failed: %v", err)
	}

	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan("접근성 에이전트 재작성", []StageDef{{
		Stage:   "code-writer",
		Keyword: "llm",
		Agent: &AgentContract{
			Name:        "ModernAccessibilityAgent",
			Description: "Builds modern accessible interfaces.",
			Spec: AgentSpec{
				Role:         "frontend accessibility engineer",
				Capabilities: []string{"accessibility", "design-system"},
			},
		},
		AgentDecision: &AgentLifecycleDecision{
			Action:            AgentPolicyRewrite,
			ExistingAgentName: "LegacyAccessibilityAgent",
			Reason:            "LLM selected rewrite",
		},
	}})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, staticAnalyzer{result: AnalysisResult{
		Intent:     IntentResult{Category: IntentCodeWriter, Confidence: 0.91},
		Complexity: ComplexityResult{Level: "MEDIUM", Score: 4},
		Plan:       plan,
		Source:     AnalyzerLLM,
	}})

	run, err := tool.Run("접근성 에이전트 재작성")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	status, err := tool.GetStatus(run["runId"].(string))
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	stage := status["stages"].([]StageResult)[0]
	if stage.Agent == nil || stage.Agent.Name != "ModernAccessibilityAgent" {
		t.Fatalf("expected analyzer rewrite candidate to be selected, got %#v", stage.Agent)
	}
	if stage.AgentDecision == nil || stage.AgentDecision.Action != AgentPolicyRewrite {
		t.Fatalf("expected analyzer rewrite decision, got %#v", stage.AgentDecision)
	}
	if stage.AgentDecision.Reason != "LLM selected rewrite" {
		t.Fatalf("expected analyzer decision reason to be preserved, got %#v", stage.AgentDecision)
	}
}

func TestToolListAgentsReturnsPersistedContracts(t *testing.T) {
	dataDir := t.TempDir()
	tool := NewToolWithAnalyzer(Options{DataDir: dataDir}, analyzerForAgentPlan(t, dataDir, "접근성 UI 에이전트 생성", AgentContract{
		Name: "InspectableAccessibilityAgent",
		Spec: AgentSpec{
			Role:         "frontend accessibility engineer",
			Capabilities: []string{"accessibility"},
		},
	}))
	if _, err := tool.Run("접근성 UI 에이전트 생성"); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	listed, err := tool.ListAgents()
	if err != nil {
		t.Fatalf("list agents failed: %v", err)
	}
	if listed["action"] != "list_agents" {
		t.Fatalf("expected list_agents action, got %#v", listed)
	}
	agents := listed["agents"].([]AgentContract)
	if len(agents) != 1 || agents[0].Name != "InspectableAccessibilityAgent" {
		t.Fatalf("expected persisted agent contract, got %#v", agents)
	}
}

func TestToolRefreshesLLMAgentContextBetweenRequests(t *testing.T) {
	dataDir := t.TempDir()
	userMessages := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		userMessages = append(userMessages, payload.Messages[len(payload.Messages)-1].Content)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"intent":"code-writer","confidence":0.8,"keywords":["접근성"],"complexity":{"level":"LOW","score":2},"stages":[{"stage":"code-writer","keyword":"llm","agent":{"name":"SavedAccessibilityAgent","spec":{"role":"frontend accessibility engineer","capabilities":["accessibility"]}}}]}`,
				},
			}},
		})
	}))
	defer server.Close()

	tool := NewTool(Options{
		DataDir:  dataDir,
		Analyzer: AnalyzerLLM,
		LLM: LLMAnalyzerConfig{
			Endpoint: server.URL,
			APIKey:   "test-key",
			Model:    "test-model",
		},
	})
	if _, err := tool.Run("접근성 에이전트 생성"); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if _, err := tool.Analyze("저장된 에이전트 참고해서 분석"); err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if len(userMessages) != 2 {
		t.Fatalf("expected two LLM calls, got %#v", userMessages)
	}
	if !strings.Contains(userMessages[1], "SavedAccessibilityAgent") {
		t.Fatalf("expected second LLM request to include refreshed agent context, got %s", userMessages[1])
	}
}

func analyzerForAgentPlan(t *testing.T, dataDir string, request string, agent AgentContract) Analyzer {
	t.Helper()
	orchestrator := NewOrchestrator(dataDir)
	plan, err := orchestrator.NormalizePlan(request, []StageDef{{
		Stage:   "code-writer",
		Keyword: "llm",
		Agent:   &agent,
	}})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	return staticAnalyzer{
		result: AnalysisResult{
			Intent:     IntentResult{Category: IntentCodeWriter, Confidence: 0.91},
			Complexity: ComplexityResult{Level: "MEDIUM", Score: 4},
			Plan:       plan,
			Source:     AnalyzerLLM,
		},
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
