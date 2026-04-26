package verdandi

import "os"

const (
	AnalyzerKeyword = "keyword"
	AnalyzerLLM     = "llm"
	AnalyzerAuto    = "auto"
)

type Analyzer interface {
	Analyze(request string) (AnalysisResult, error)
}

type AnalysisResult struct {
	Text       string             `json:"text"`
	Intent     IntentResult       `json:"intent"`
	Complexity ComplexityResult   `json:"complexity"`
	Keywords   []KeywordFrequency `json:"keywords"`
	Plan       Plan               `json:"plan"`
	Source     string             `json:"source"`
}

type AnalyzerConfig struct {
	Mode         string
	Orchestrator Orchestrator
	LLM          LLMAnalyzerConfig
	Fallback     Analyzer
}

func NewAnalyzer(config AnalyzerConfig) Analyzer {
	mode := config.Mode
	if mode == "" {
		mode = os.Getenv("VERDANDI_ANALYZER")
	}
	if mode == "" {
		mode = AnalyzerKeyword
	}

	fallback := config.Fallback
	if fallback == nil {
		fallback = NewKeywordAnalyzer(config.Orchestrator)
	}

	switch mode {
	case AnalyzerLLM:
		return NewLLMAnalyzer(config.LLM, fallback)
	case AnalyzerAuto:
		if config.LLM.APIKey != "" || os.Getenv("VERDANDI_LLM_API_KEY") != "" {
			return NewLLMAnalyzer(config.LLM, fallback)
		}
		return fallback
	default:
		return fallback
	}
}
