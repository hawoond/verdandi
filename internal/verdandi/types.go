package verdandi

import "time"

const (
	IntentCodeWriter   = "code-writer"
	IntentDocumenter   = "documenter"
	IntentResearcher   = "researcher"
	IntentDataAnalyst  = "data-analyst"
	IntentPlanner      = "planner"
	IntentOrchestrator = "orchestrator"
	IntentGeneral      = "general"
)

type Analysis struct {
	Text       string             `json:"text"`
	Intent     IntentResult       `json:"intent"`
	Complexity ComplexityResult   `json:"complexity"`
	Keywords   []KeywordFrequency `json:"keywords"`
}

type IntentResult struct {
	Category   string   `json:"category"`
	Confidence float64  `json:"confidence"`
	Keywords   []string `json:"keywords"`
}

type ComplexityResult struct {
	Level string `json:"level"`
	Score int    `json:"score"`
}

type KeywordFrequency struct {
	Word      string `json:"word"`
	Frequency int    `json:"frequency"`
}

type AgentContract struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Command     string            `json:"command"`
	Spec        AgentSpec         `json:"spec"`
	Metadata    map[string]any    `json:"metadata"`
	Inputs      map[string]string `json:"inputs"`
}

type AgentSpec struct {
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
}

type StageDef struct {
	Stage   string `json:"stage"`
	Keyword string `json:"keyword"`
	Order   int    `json:"order"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type ExecutionGraph struct {
	Nodes []string `json:"nodes"`
	Edges []Edge   `json:"edges"`
}

type Plan struct {
	OriginalRequest string         `json:"originalRequest"`
	Stages          []StageDef     `json:"stages"`
	StageCount      int            `json:"stageCount"`
	Graph           ExecutionGraph `json:"graph"`
}

type StageResult struct {
	Stage   string       `json:"stage"`
	Status  string       `json:"status"`
	Result  *StageOutput `json:"result,omitempty"`
	Error   string       `json:"error,omitempty"`
	Started time.Time    `json:"started_at"`
	Ended   time.Time    `json:"ended_at"`
}

type StageOutput struct {
	Type      string       `json:"type"`
	Status    string       `json:"status,omitempty"`
	Message   string       `json:"message,omitempty"`
	OutputDir string       `json:"outputDir,omitempty"`
	Files     []FileEntry  `json:"files,omitempty"`
	Tests     []TestResult `json:"tests,omitempty"`
}

type TestResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Output string `json:"output,omitempty"`
}

type Summary struct {
	TotalStages int         `json:"totalStages"`
	Success     int         `json:"success"`
	Failed      int         `json:"failed"`
	OutputDir   string      `json:"outputDir,omitempty"`
	Files       []FileEntry `json:"files"`
}

type ExecutionResult struct {
	Request     string        `json:"request"`
	Plan        Plan          `json:"plan"`
	Analyzer    string        `json:"analyzer,omitempty"`
	Stages      []StageResult `json:"stages"`
	OutputDir   string        `json:"outputDir,omitempty"`
	Summary     Summary       `json:"summary"`
	CompletedAt time.Time     `json:"completed_at"`
}

type FileEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Status string `json:"status"`
}

type FileInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	IsDirectory bool   `json:"isDirectory"`
}

type RunRecord struct {
	RunID       string        `json:"runId"`
	Status      string        `json:"status"`
	Request     string        `json:"request"`
	Analyzer    string        `json:"analyzer,omitempty"`
	OutputDir   string        `json:"outputDir,omitempty"`
	Summary     Summary       `json:"summary"`
	Stages      []StageResult `json:"stages"`
	CreatedAt   time.Time     `json:"created_at"`
	CompletedAt time.Time     `json:"completed_at"`
}
