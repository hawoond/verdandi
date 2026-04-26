package verdandi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type LLMAnalyzerConfig struct {
	Endpoint string
	Model    string
	APIKey   string
	Timeout  time.Duration
	Client   *http.Client
}

type LLMAnalyzer struct {
	config   LLMAnalyzerConfig
	fallback Analyzer
}

type llmPlanResponse struct {
	Intent     string           `json:"intent"`
	Confidence float64          `json:"confidence"`
	Keywords   []string         `json:"keywords"`
	Complexity ComplexityResult `json:"complexity"`
	Stages     []StageDef       `json:"stages"`
}

func NewLLMAnalyzer(config LLMAnalyzerConfig, fallback Analyzer) LLMAnalyzer {
	return LLMAnalyzer{config: config, fallback: fallback}
}

func (a LLMAnalyzer) Analyze(request string) (AnalysisResult, error) {
	result, err := a.analyzeWithLLM(request)
	if err == nil {
		return result, nil
	}
	if a.fallback != nil {
		fallback, fallbackErr := a.fallback.Analyze(request)
		if fallbackErr != nil {
			return AnalysisResult{}, fallbackErr
		}
		fallback.FallbackReason = err.Error()
		return fallback, nil
	}
	return AnalysisResult{}, err
}

func (a LLMAnalyzer) analyzeWithLLM(request string) (AnalysisResult, error) {
	endpoint := firstNonEmpty(a.config.Endpoint, os.Getenv("VERDANDI_LLM_ENDPOINT"))
	apiKey := firstNonEmpty(a.config.APIKey, os.Getenv("VERDANDI_LLM_API_KEY"))
	model := firstNonEmpty(a.config.Model, os.Getenv("VERDANDI_LLM_MODEL"), "gpt-4o-mini")
	if endpoint == "" || apiKey == "" {
		return AnalysisResult{}, fmt.Errorf("llm analyzer requires endpoint and api key")
	}

	timeout := a.config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	client := a.config.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": llmAnalyzerSystemPrompt()},
			{"role": "user", "content": request},
		},
		"temperature": 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return AnalysisResult{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return AnalysisResult{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpRes, err := client.Do(httpReq)
	if err != nil {
		return AnalysisResult{}, err
	}
	defer httpRes.Body.Close()
	if httpRes.StatusCode < 200 || httpRes.StatusCode >= 300 {
		return AnalysisResult{}, fmt.Errorf("llm analyzer status: %s", httpRes.Status)
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(httpRes.Body).Decode(&decoded); err != nil {
		return AnalysisResult{}, err
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return AnalysisResult{}, fmt.Errorf("llm analyzer returned empty content")
	}

	var proposed llmPlanResponse
	if err := json.Unmarshal([]byte(decoded.Choices[0].Message.Content), &proposed); err != nil {
		return AnalysisResult{}, err
	}
	orchestrator := NewOrchestrator("")
	plan, err := orchestrator.NormalizePlan(request, proposed.Stages)
	if err != nil {
		return AnalysisResult{}, err
	}

	return AnalysisResult{
		Text: request,
		Intent: IntentResult{
			Category:   normalizeIntent(proposed.Intent),
			Confidence: clampConfidence(proposed.Confidence),
			Keywords:   proposed.Keywords,
		},
		Complexity: proposed.Complexity,
		Keywords:   keywordFrequencies(proposed.Keywords),
		Plan:       plan,
		Source:     AnalyzerLLM,
	}, nil
}

func llmAnalyzerSystemPrompt() string {
	return `Return only JSON. Choose intent from code-writer, documenter, researcher, data-analyst, planner, orchestrator, general. Choose stages only from planner, code-writer, tester, documenter, deployer. Include confidence from 0 to 1, keywords, complexity with level LOW/MEDIUM/HIGH and score 0-10, and stages.`
}

func normalizeIntent(intent string) string {
	switch intent {
	case IntentCodeWriter, IntentDocumenter, IntentResearcher, IntentDataAnalyst, IntentPlanner, IntentOrchestrator, IntentGeneral:
		return intent
	default:
		return IntentGeneral
	}
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func keywordFrequencies(words []string) []KeywordFrequency {
	result := make([]KeywordFrequency, 0, len(words))
	seen := map[string]bool{}
	for _, word := range words {
		word = strings.TrimSpace(strings.ToLower(word))
		if word == "" || seen[word] {
			continue
		}
		result = append(result, KeywordFrequency{Word: word, Frequency: 1})
		seen[word] = true
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
