package verdandi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLLMAnalyzerParsesPlanFromCompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing bearer token: %s", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"intent":"orchestrator","confidence":0.91,"keywords":["계산기","구현"],"complexity":{"level":"MEDIUM","score":4},"stages":[{"stage":"planner","keyword":"llm"},{"stage":"code-writer","keyword":"llm"},{"stage":"documenter","keyword":"llm"}]}`,
				},
			}},
		})
	}))
	defer server.Close()

	analyzer := NewLLMAnalyzer(LLMAnalyzerConfig{
		Endpoint: server.URL,
		Model:    "test-model",
		APIKey:   "test-key",
	}, NewKeywordAnalyzer(NewOrchestrator(t.TempDir())))

	result, err := analyzer.Analyze("계산기 앱 만들어줘")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if result.Source != AnalyzerLLM {
		t.Fatalf("expected llm source, got %s", result.Source)
	}
	if result.Intent.Category != IntentOrchestrator {
		t.Fatalf("unexpected intent: %#v", result.Intent)
	}
	got := []string{}
	for _, stage := range result.Plan.Stages {
		got = append(got, stage.Stage)
	}
	want := []string{"planner", "code-writer", "tester", "documenter"}
	if !equalStrings(got, want) {
		t.Fatalf("stages mismatch: got %#v want %#v", got, want)
	}
}

func TestLLMAnalyzerFallsBackWhenOutputIsInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"intent":"orchestrator","confidence":0.91,"stages":[{"stage":"shell","keyword":"bad"}]}`,
				},
			}},
		})
	}))
	defer server.Close()

	analyzer := NewLLMAnalyzer(LLMAnalyzerConfig{
		Endpoint: server.URL,
		Model:    "test-model",
		APIKey:   "test-key",
	}, NewKeywordAnalyzer(NewOrchestrator(t.TempDir())))

	result, err := analyzer.Analyze("기획 구현 테스트 문서화")
	if err != nil {
		t.Fatalf("fallback analyze failed: %v", err)
	}
	if result.Source != AnalyzerKeyword {
		t.Fatalf("expected keyword fallback source, got %s", result.Source)
	}
	if result.FallbackReason == "" {
		t.Fatalf("expected fallback reason")
	}
	if result.Plan.StageCount != 4 {
		t.Fatalf("expected keyword fallback plan, got %#v", result.Plan)
	}
}

func TestLLMAnalyzerParsesFencedJSONContent(t *testing.T) {
	content := "```json\n" +
		`{"intent":"planner","confidence":0.8,"keywords":["기획"],"complexity":{"level":"LOW","score":1},"stages":[{"stage":"planner","keyword":"llm"}]}` +
		"\n```"
	result := analyzeWithLLMContent(t, content, "기획해줘")

	if result.Source != AnalyzerLLM {
		t.Fatalf("expected llm source, got %s", result.Source)
	}
	if result.Intent.Category != IntentPlanner {
		t.Fatalf("expected planner intent, got %#v", result.Intent)
	}
	if result.Plan.StageCount != 1 || result.Plan.Stages[0].Stage != "planner" {
		t.Fatalf("expected planner-only plan, got %#v", result.Plan)
	}
}

func TestLLMAnalyzerExtractsJSONFromMixedContent(t *testing.T) {
	content := `Here is the plan: {"intent":"documenter","confidence":0.7,"keywords":["문서"],"complexity":{"level":"LOW","score":1},"stages":[{"stage":"documenter","keyword":"llm"}]}`
	result := analyzeWithLLMContent(t, content, "문서 작성")

	if result.Source != AnalyzerLLM {
		t.Fatalf("expected llm source, got %s", result.Source)
	}
	if result.Intent.Category != IntentDocumenter {
		t.Fatalf("expected documenter intent, got %#v", result.Intent)
	}
	if result.Plan.StageCount != 1 || result.Plan.Stages[0].Stage != "documenter" {
		t.Fatalf("expected documenter-only plan, got %#v", result.Plan)
	}
}

func TestLLMAnalyzerFallsBackWithSpecificReasonWhenStagesMissing(t *testing.T) {
	result := analyzeWithLLMContent(t, `{"intent":"planner","confidence":0.8,"keywords":["기획"],"complexity":{"level":"LOW","score":1}}`, "기획 구현 테스트 문서화")

	if result.Source != AnalyzerKeyword {
		t.Fatalf("expected keyword fallback source, got %s", result.Source)
	}
	if !strings.Contains(result.FallbackReason, "missing stages") {
		t.Fatalf("expected missing stages fallback reason, got %q", result.FallbackReason)
	}
}

func TestLLMAnalyzerParsesDynamicAgentContract(t *testing.T) {
	content := `{
		"intent":"code-writer",
		"confidence":0.88,
		"keywords":["접근성","계산기"],
		"complexity":{"level":"MEDIUM","score":4},
		"stages":[{
			"stage":"code-writer",
			"keyword":"llm",
			"agent":{
				"name":"AccessibilityFocusedFrontendAgent",
				"description":"Builds UI code with accessibility checks.",
				"spec":{
					"role":"frontend accessibility engineer",
					"capabilities":["ui-implementation","accessibility","validation"]
				}
			}
		}]
	}`
	result := analyzeWithLLMContent(t, content, "접근성 좋은 계산기 앱 구현")

	if result.Source != AnalyzerLLM {
		t.Fatalf("expected llm source, got %s", result.Source)
	}
	if result.Plan.Stages[0].Agent == nil {
		t.Fatalf("expected dynamic agent in stage: %#v", result.Plan.Stages[0])
	}
	agent := result.Plan.Stages[0].Agent
	if agent.Name != "AccessibilityFocusedFrontendAgent" {
		t.Fatalf("unexpected agent contract: %#v", agent)
	}
	if agent.Spec.Role != "frontend accessibility engineer" {
		t.Fatalf("unexpected agent role: %#v", agent.Spec)
	}
	if len(agent.Spec.Capabilities) != 3 {
		t.Fatalf("unexpected capabilities: %#v", agent.Spec.Capabilities)
	}
}

func TestLLMAnalyzerSendsExistingAgentContext(t *testing.T) {
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": `{"intent":"code-writer","confidence":0.8,"keywords":["접근성"],"complexity":{"level":"LOW","score":2},"stages":[{"stage":"code-writer","keyword":"llm"}]}`,
				},
			}},
		})
	}))
	defer server.Close()

	analyzer := NewLLMAnalyzer(LLMAnalyzerConfig{
		Endpoint: server.URL,
		Model:    "test-model",
		APIKey:   "test-key",
		ExistingAgents: []AgentContract{{
			Name: "ExistingAccessibilityAgent",
			Spec: AgentSpec{
				Role:         "frontend accessibility engineer",
				Capabilities: []string{"accessibility"},
			},
		}},
	}, NewKeywordAnalyzer(NewOrchestrator(t.TempDir())))

	if _, err := analyzer.Analyze("접근성 좋은 UI 구현"); err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if len(payload.Messages) < 2 {
		t.Fatalf("expected messages in payload, got %#v", payload.Messages)
	}
	userContent := payload.Messages[len(payload.Messages)-1].Content
	systemContent := payload.Messages[0].Content
	if !strings.Contains(systemContent, "metrics") || !strings.Contains(systemContent, "lifecycleRecommendation") {
		t.Fatalf("expected system prompt to instruct performance-aware lifecycle decisions, got %s", systemContent)
	}
	if !strings.Contains(userContent, "ExistingAccessibilityAgent") {
		t.Fatalf("expected existing agent context in LLM payload, got %s", userContent)
	}
	if !strings.Contains(userContent, AgentPolicyReuseEnhance) || !strings.Contains(userContent, AgentPolicyRewrite) || !strings.Contains(userContent, AgentPolicySeparate) {
		t.Fatalf("expected lifecycle options in LLM payload, got %s", userContent)
	}
}

func TestLLMAnalyzerParsesAgentLifecycleDecision(t *testing.T) {
	content := `{
		"intent":"code-writer",
		"confidence":0.88,
		"keywords":["접근성","계산기"],
		"complexity":{"level":"MEDIUM","score":4},
		"stages":[{
			"stage":"code-writer",
			"keyword":"llm",
			"agent":{
				"name":"ModernAccessibilityAgent",
				"spec":{
					"role":"frontend accessibility engineer",
					"capabilities":["ui-implementation","accessibility"]
				}
			},
			"agentDecision":{
				"action":"rewrite",
				"existingAgentName":"LegacyAccessibilityAgent",
				"reason":"legacy contract is too narrow"
			}
		}]
	}`
	result := analyzeWithLLMContent(t, content, "접근성 좋은 계산기 앱 구현")

	decision := result.Plan.Stages[0].AgentDecision
	if decision == nil {
		t.Fatalf("expected lifecycle decision in plan: %#v", result.Plan.Stages[0])
	}
	if decision.Action != AgentPolicyRewrite || decision.ExistingAgentName != "LegacyAccessibilityAgent" {
		t.Fatalf("unexpected lifecycle decision: %#v", decision)
	}
}

func TestLLMAnalyzerParsesWorkflowAssets(t *testing.T) {
	content := `{
		"intent": "orchestrator",
		"confidence": 0.91,
		"keywords": ["go", "cli", "calculator", "tdd"],
		"complexity": {"level": "MEDIUM", "score": 4},
		"stages": [
			{"stage": "planner", "keyword": "plan"},
			{"stage": "code-writer", "keyword": "implement"},
			{"stage": "tester", "keyword": "verify"}
		],
		"agents": [
			{
				"name": "GoCalculatorImplementer",
				"role": "code-writer",
				"capabilities": ["go", "cli", "calculator", "tdd"]
			}
		],
		"skills": [
			{
				"name": "go-cli-calculator-tdd",
				"description": "Build Go calculator CLIs with tests first.",
				"whenToUse": "When implementing Go calculator CLI behavior.",
				"instructions": "Write failing tests for add, subtract, multiply, divide before implementation.",
				"inputs": ["request", "acceptanceCriteria"],
				"outputs": ["patch", "testResults"],
				"constraints": ["External coding agent writes code, not Verdandi."]
			}
		]
	}`
	result := analyzeWithLLMContent(t, content, "Go calculator CLI를 TDD로 구현")

	if len(result.WorkflowAssets.Agents) != 1 {
		t.Fatalf("expected one workflow agent asset, got %#v", result.WorkflowAssets.Agents)
	}
	agent := result.WorkflowAssets.Agents[0]
	if agent.Name != "GoCalculatorImplementer" || agent.Role != "code-writer" {
		t.Fatalf("unexpected workflow agent: %#v", agent)
	}
	if agent.Contract.Spec.Role != "code-writer" || !equalStrings(agent.Contract.Spec.Capabilities, []string{"go", "cli", "calculator", "tdd"}) {
		t.Fatalf("unexpected agent contract: %#v", agent.Contract)
	}
	if agent.ID == "" || agent.Status != AssetStatusActive {
		t.Fatalf("expected normalized active agent asset, got %#v", agent)
	}

	if len(result.WorkflowAssets.Skills) != 1 {
		t.Fatalf("expected one workflow skill asset, got %#v", result.WorkflowAssets.Skills)
	}
	skill := result.WorkflowAssets.Skills[0]
	if skill.Name != "go-cli-calculator-tdd" || skill.Contract.Name != "go-cli-calculator-tdd" {
		t.Fatalf("unexpected workflow skill: %#v", skill)
	}
	if !strings.Contains(skill.Contract.Instructions, "failing tests") {
		t.Fatalf("expected skill instructions to be preserved, got %#v", skill.Contract)
	}
	if skill.ID == "" || skill.Status != AssetStatusActive {
		t.Fatalf("expected normalized active skill asset, got %#v", skill)
	}
}

func TestLLMAnalyzerParsesWorkflowAgentContractShape(t *testing.T) {
	content := `{
		"intent": "orchestrator",
		"confidence": 0.91,
		"keywords": ["go", "cli"],
		"complexity": {"level": "MEDIUM", "score": 4},
		"stages": [
			{"stage": "code-writer", "keyword": "implement"}
		],
		"agents": [
			{
				"name": "GoCLIContractAgent",
				"description": "Builds Go CLIs from existing repo patterns.",
				"command": "codex",
				"spec": {
					"role": "go cli engineer",
					"capabilities": ["go", "cli", "tests"]
				}
			}
		]
	}`
	result := analyzeWithLLMContent(t, content, "Go CLI 구현")

	if len(result.WorkflowAssets.Agents) != 1 {
		t.Fatalf("expected one workflow agent asset, got %#v", result.WorkflowAssets.Agents)
	}
	agent := result.WorkflowAssets.Agents[0]
	if agent.Role != "go cli engineer" || agent.Contract.Spec.Role != "go cli engineer" {
		t.Fatalf("expected contract-shaped role to be preserved, got %#v", agent)
	}
	if !equalStrings(agent.Contract.Spec.Capabilities, []string{"go", "cli", "tests"}) {
		t.Fatalf("expected contract-shaped capabilities to be preserved, got %#v", agent.Contract.Spec.Capabilities)
	}
}

func analyzeWithLLMContent(t *testing.T, content string, request string) AnalysisResult {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": content,
				},
			}},
		})
	}))
	defer server.Close()

	analyzer := NewLLMAnalyzer(LLMAnalyzerConfig{
		Endpoint: server.URL,
		Model:    "test-model",
		APIKey:   "test-key",
	}, NewKeywordAnalyzer(NewOrchestrator(t.TempDir())))

	result, err := analyzer.Analyze(request)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	return result
}
