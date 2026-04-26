package verdandi

type KeywordAnalyzer struct {
	classifier   Classifier
	orchestrator Orchestrator
}

func NewKeywordAnalyzer(orchestrator Orchestrator) KeywordAnalyzer {
	return KeywordAnalyzer{
		classifier:   NewClassifier(),
		orchestrator: orchestrator,
	}
}

func (a KeywordAnalyzer) Analyze(request string) (AnalysisResult, error) {
	analysis := a.classifier.Analyze(request)
	return AnalysisResult{
		Text:       analysis.Text,
		Intent:     analysis.Intent,
		Complexity: analysis.Complexity,
		Keywords:   analysis.Keywords,
		Plan:       a.orchestrator.ParseRequest(request),
		Source:     AnalyzerKeyword,
	}, nil
}
