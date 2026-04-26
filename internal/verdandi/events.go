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
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	file, err := os.Create(s.path(record.RunID))
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	return nil
}

func (s EventStore) List(runID string) ([]VisualizationEvent, error) {
	file, err := os.Open(s.path(runID))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	events := []VisualizationEvent{}
	scanner := bufio.NewScanner(file)
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
	events := []VisualizationEvent{{
		RunID:     record.RunID,
		Type:      EventRunStarted,
		Timestamp: record.CreatedAt,
		Status:    record.Status,
		Message:   record.Request,
	}}

	for _, stage := range record.Stages {
		agent := visualizationAgentForStage(stage)
		events = append(events, VisualizationEvent{
			RunID:     record.RunID,
			Type:      EventAgentSpawned,
			Timestamp: stage.Started,
			Stage:     stage.Stage,
			Agent:     agent,
			Message:   "Hello, I'm ready.",
		})
		events = append(events, VisualizationEvent{
			RunID:     record.RunID,
			Type:      EventStageStarted,
			Timestamp: stage.Started,
			Stage:     stage.Stage,
			Agent:     agent,
			Message:   stageMovementMessage(stage.Stage),
		})
		if stage.AgentDecision != nil {
			events = append(events, VisualizationEvent{
				RunID:     record.RunID,
				Type:      EventAgentDecision,
				Timestamp: stage.Started,
				Stage:     stage.Stage,
				Agent:     agent,
				Decision:  stage.AgentDecision,
				Message:   stage.AgentDecision.Reason,
			})
		}
		events = append(events, VisualizationEvent{
			RunID:     record.RunID,
			Type:      EventStageCompleted,
			Timestamp: stage.Ended,
			Stage:     stage.Stage,
			Status:    stage.Status,
			Agent:     agent,
			Message:   stage.Error,
		})
		if stage.Agent != nil {
			metrics := stage.Agent.Metrics
			events = append(events, VisualizationEvent{
				RunID:     record.RunID,
				Type:      EventMetricsUpdated,
				Timestamp: stage.Ended,
				Stage:     stage.Stage,
				Status:    stage.Status,
				Agent:     agent,
				Metrics:   &metrics,
				Message:   "agent metrics updated",
			})
		}
	}

	events = append(events, VisualizationEvent{
		RunID:     record.RunID,
		Type:      EventRunCompleted,
		Timestamp: record.CompletedAt,
		Status:    record.Status,
		Message:   "run completed",
	})
	return events
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
