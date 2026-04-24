package verdandi

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type Classifier struct {
	keywords map[string][]string
}

func NewClassifier() Classifier {
	return Classifier{keywords: map[string][]string{
		IntentCodeWriter: {
			"코드", "구현", "컴포넌트", "함수", "클래스", "메서드",
			"react", "vue", "angular", "typescript", "javascript", "프로퍼티", "상태", "이벤트",
		},
		IntentDocumenter: {
			"문서", "readme", "기획서", "설계", "명세서", "스펙", "요구사항", "가이드", "튜토리얼", "참고",
		},
		IntentResearcher: {
			"찾기", "검색", "조사", "분석", "조회", "엔드포인트", "api", "데이터베이스", "구조", "탐색",
		},
		IntentDataAnalyst: {
			"통계", "분석", "차트", "리포트", "지표", "kpi", "메트릭", "수치", "비교", "추이",
		},
		IntentPlanner: {
			"기획", "계획", "로드맵", "일정", "스케줄", "전략", "방안", "프로젝트", "마일스톤", "체크리스트",
		},
		IntentOrchestrator: {
			"작업", "에이전트", "동적", "생성", "연계", "실행", "오케스트레이션", "오케스트레이터",
			"파이프라인", "워크플로우", "분해", "분할", "조율", "체인", "병렬", "순차",
		},
	}}
}

func (c Classifier) Analyze(text string) Analysis {
	keywords := c.ExtractKeywords(text)
	intent := c.ClassifyIntent(text, keywords)
	complexity := c.EvaluateComplexity(text)

	return Analysis{
		Text:       text,
		Intent:     intent,
		Complexity: complexity,
		Keywords:   keywords,
	}
}

func (c Classifier) ClassifyIntent(text string, keywords []KeywordFrequency) IntentResult {
	processed := preprocess(text)
	keywordSet := map[string]bool{}
	for _, keyword := range keywords {
		keywordSet[keyword.Word] = true
	}

	bestIntent := IntentGeneral
	bestScore := 0
	for intent, intentKeywords := range c.keywords {
		score := 0
		for _, keyword := range intentKeywords {
			normalized := strings.ToLower(keyword)
			if keywordSet[normalized] || strings.Contains(processed, normalized) {
				score++
			}
		}
		if score > bestScore {
			bestIntent = intent
			bestScore = score
		}
	}

	top := make([]string, 0, min(5, len(keywords)))
	for i := 0; i < len(keywords) && i < 5; i++ {
		top = append(top, keywords[i].Word)
	}

	if bestScore == 0 {
		return IntentResult{Category: IntentGeneral, Confidence: 0, Keywords: top}
	}

	confidence := float64(bestScore) / 5.0
	if confidence > 1 {
		confidence = 1
	}

	return IntentResult{Category: bestIntent, Confidence: confidence, Keywords: top}
}

func (c Classifier) ExtractKeywords(text string) []KeywordFrequency {
	tokens := tokenRegexp.FindAllString(preprocess(text), -1)
	counts := map[string]int{}
	for _, token := range tokens {
		minLength := 3
		if containsKorean(token) {
			minLength = 2
		}
		if utf8.RuneCountInString(token) >= minLength {
			counts[token]++
		}
	}

	keywords := make([]KeywordFrequency, 0, len(counts))
	for word, frequency := range counts {
		keywords = append(keywords, KeywordFrequency{Word: word, Frequency: frequency})
	}
	sort.Slice(keywords, func(i, j int) bool {
		if keywords[i].Frequency == keywords[j].Frequency {
			return keywords[i].Word < keywords[j].Word
		}
		return keywords[i].Frequency > keywords[j].Frequency
	})
	if len(keywords) > 20 {
		return keywords[:20]
	}
	return keywords
}

func (c Classifier) EvaluateComplexity(text string) ComplexityResult {
	processed := preprocess(text)
	score := 0

	for _, keyword := range []string{"단순", "기본", "초기", "작은"} {
		if strings.Contains(processed, keyword) {
			score++
		}
	}
	for _, keyword := range []string{"중간", "복합", "여러", "다양한"} {
		if strings.Contains(processed, keyword) {
			score += 2
		}
	}
	for _, keyword := range []string{"고급", "복잡", "대규모", "심층", "전체"} {
		if strings.Contains(processed, keyword) {
			score += 3
		}
	}
	if len(strings.Fields(processed)) > 12 {
		score += 2
	}
	if score > 10 {
		score = 10
	}

	level := "LOW"
	if score >= 5 {
		level = "HIGH"
	} else if score >= 3 {
		level = "MEDIUM"
	}

	return ComplexityResult{Level: level, Score: score}
}

var tokenRegexp = regexp.MustCompile(`[a-z0-9]+|[가-힣]+`)
var stripRegexp = regexp.MustCompile(`[^\w\s가-힣]`)

func preprocess(text string) string {
	return strings.TrimSpace(stripRegexp.ReplaceAllString(strings.ToLower(text), ""))
}

func containsKorean(text string) bool {
	for _, r := range text {
		if r >= '가' && r <= '힣' {
			return true
		}
	}
	return false
}
