package verdandi

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	EventRunStarted     = "run-started"
	EventRunCompleted   = "run-completed"
	EventAgentSpawned   = "agent-spawned"
	EventAgentDecision  = "agent-decision"
	EventStageStarted   = "stage-started"
	EventStageCompleted = "stage-completed"
	EventMetricsUpdated = "metrics-updated"
)

const maxEventLineBytes = 10 * 1024 * 1024

type VisualizationEvent struct {
	RunID     string                  `json:"runId"`
	Type      string                  `json:"type"`
	Timestamp time.Time               `json:"timestamp"`
	Stage     string                  `json:"stage,omitempty"`
	Status    string                  `json:"status,omitempty"`
	Message   string                  `json:"message,omitempty"`
	Agent     *VisualizationAgent     `json:"agent,omitempty"`
	Decision  *AgentLifecycleDecision `json:"decision,omitempty"`
	Metrics   *AgentMetrics           `json:"metrics,omitempty"`
}

type VisualizationAgent struct {
	Name         string              `json:"name"`
	Role         string              `json:"role"`
	Capabilities []string            `json:"capabilities,omitempty"`
	Avatar       VisualizationAvatar `json:"avatar"`
}

type VisualizationAvatar struct {
	Kind string `json:"kind"`
}

type EventStore struct {
	dir string
}

func NewEventStoreForDataDir(dataDir string) EventStore {
	return EventStore{dir: filepath.Join(dataDir, "events")}
}

func (s EventStore) SaveRun(record RunRecord) error {
	events := eventsForRun(record)
	return s.Save(record.RunID, events)
}

func (s EventStore) Save(runID string, events []VisualizationEvent) error {
	path := s.path(runID)
	lock := lockForPath(path)
	lock.Lock()
	defer lock.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.dir, runID+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	encoder := json.NewEncoder(file)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func (s EventStore) Reset(runID string) error {
	return s.Save(runID, []VisualizationEvent{})
}

func (s EventStore) Append(event VisualizationEvent) error {
	path := s.path(event.RunID)
	lock := lockForPath(path)
	lock.Lock()
	defer lock.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(event)
}

func (s EventStore) AppendMany(events []VisualizationEvent) error {
	for _, event := range events {
		if err := s.Append(event); err != nil {
			return err
		}
	}
	return nil
}

func (s EventStore) List(runID string) ([]VisualizationEvent, error) {
	path := s.path(runID)
	lock := lockForPath(path)
	lock.Lock()
	defer lock.Unlock()

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	events := []VisualizationEvent{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxEventLineBytes)
	for scanner.Scan() {
		var event VisualizationEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s EventStore) path(runID string) string {
	return filepath.Join(s.dir, runID+".jsonl")
}

func eventsForRun(record RunRecord) []VisualizationEvent {
	events := []VisualizationEvent{runStartedEvent(record.RunID, record.Status, record.Request, record.CreatedAt)}

	for _, stage := range record.Stages {
		events = append(events, stageStartedEvents(record.RunID, stage)...)
		events = append(events, stageCompletedEvents(record.RunID, stage)...)
		events = append(events, metricsUpdatedEvents(record.RunID, stage)...)
	}

	events = append(events, runCompletedEvent(record.RunID, record.Status, record.CompletedAt))
	return events
}

func runStartedEvent(runID string, status string, request string, timestamp time.Time) VisualizationEvent {
	return VisualizationEvent{
		RunID:     runID,
		Type:      EventRunStarted,
		Timestamp: timestamp,
		Status:    status,
		Message:   request,
	}
}

func stageStartedEvents(runID string, stage StageResult) []VisualizationEvent {
	agent := visualizationAgentForStage(stage)
	events := []VisualizationEvent{{
		RunID:     runID,
		Type:      EventAgentSpawned,
		Timestamp: stage.Started,
		Stage:     stage.Stage,
		Agent:     agent,
		Message:   "Hello, I'm ready.",
	}, {
		RunID:     runID,
		Type:      EventStageStarted,
		Timestamp: stage.Started,
		Stage:     stage.Stage,
		Agent:     agent,
		Message:   stageMovementMessage(stage.Stage),
	}}
	if stage.AgentDecision != nil {
		events = append(events, VisualizationEvent{
			RunID:     runID,
			Type:      EventAgentDecision,
			Timestamp: stage.Started,
			Stage:     stage.Stage,
			Agent:     agent,
			Decision:  stage.AgentDecision,
			Message:   stage.AgentDecision.Reason,
		})
	}
	return events
}

func stageCompletedEvents(runID string, stage StageResult) []VisualizationEvent {
	agent := visualizationAgentForStage(stage)
	return []VisualizationEvent{{
		RunID:     runID,
		Type:      EventStageCompleted,
		Timestamp: stage.Ended,
		Stage:     stage.Stage,
		Status:    stage.Status,
		Agent:     agent,
		Message:   stage.Error,
	}}
}

func metricsUpdatedEvents(runID string, stage StageResult) []VisualizationEvent {
	agent := visualizationAgentForStage(stage)
	if stage.Agent != nil {
		metrics := stage.Agent.Metrics
		return []VisualizationEvent{{
			RunID:     runID,
			Type:      EventMetricsUpdated,
			Timestamp: stage.Ended,
			Stage:     stage.Stage,
			Status:    stage.Status,
			Agent:     agent,
			Metrics:   &metrics,
			Message:   "agent metrics updated",
		}}
	}
	return nil
}

func runCompletedEvent(runID string, status string, timestamp time.Time) VisualizationEvent {
	return VisualizationEvent{
		RunID:     runID,
		Type:      EventRunCompleted,
		Timestamp: timestamp,
		Status:    status,
		Message:   "run completed",
	}
}

func visualizationAgentForStage(stage StageResult) *VisualizationAgent {
	if agent := visualizationAgent(stage.Agent); agent != nil {
		return agent
	}
	contract := AgentContract{
		Name: fallbackVisualizationAgentName(stage.Stage),
		Spec: AgentSpec{
			Role:         stage.Stage,
			Capabilities: capabilitiesFor(stage.Stage),
		},
	}
	return visualizationAgent(&contract)
}

func visualizationAgent(agent *AgentContract) *VisualizationAgent {
	if agent == nil {
		return nil
	}
	return &VisualizationAgent{
		Name:         agent.Name,
		Role:         agent.Spec.Role,
		Capabilities: append([]string{}, agent.Spec.Capabilities...),
		Avatar:       VisualizationAvatar{Kind: animalAvatarKind(*agent)},
	}
}

func fallbackVisualizationAgentName(stage string) string {
	switch stage {
	case "planner":
		return "VerdandiPlannerCat"
	case "code-writer":
		return "VerdandiCoderDog"
	case "tester":
		return "VerdandiTesterRabbit"
	case "documenter":
		return "VerdandiDocumenterFox"
	case "deployer":
		return "VerdandiDeployPenguin"
	default:
		return "VerdandiAgent"
	}
}

func stageMovementMessage(stage string) string {
	switch stage {
	case "planner":
		return "I'm heading to the planning desk."
	case "code-writer":
		return "Moving to the coding table."
	case "tester":
		return "Hopping over to the testing lab."
	case "documenter":
		return "Bringing notes to the docs library."
	case "deployer":
		return "Waddling toward the deploy gate."
	default:
		return "Moving to the next task."
	}
}

func animalAvatarKind(agent AgentContract) string {
	text := strings.ToLower(agent.Name + " " + agent.Spec.Role + " " + strings.Join(agent.Spec.Capabilities, " "))
	switch {
	case strings.Contains(text, "cat"):
		return "cat"
	case strings.Contains(text, "dog"):
		return "dog"
	case strings.Contains(text, "rabbit"):
		return "rabbit"
	case strings.Contains(text, "fox"):
		return "fox"
	case strings.Contains(text, "penguin"):
		return "penguin"
	case strings.Contains(text, "test") || strings.Contains(text, "validation"):
		return "rabbit"
	case strings.Contains(text, "document") || strings.Contains(text, "readme"):
		return "fox"
	case strings.Contains(text, "deploy"):
		return "penguin"
	case strings.Contains(text, "code") || strings.Contains(text, "engineer"):
		return "dog"
	case strings.Contains(text, "plan"):
		return "cat"
	default:
		kinds := []string{"cat", "dog", "rabbit", "fox", "penguin"}
		sum := 0
		for _, ch := range agent.Name {
			sum += int(ch)
		}
		return kinds[sum%len(kinds)]
	}
}
