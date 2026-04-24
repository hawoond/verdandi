package verdandi

import "testing"

func TestParseRequestBuildsExecutionGraph(t *testing.T) {
	plan := NewOrchestrator("").ParseRequest("기획 구현 테스트 문서화")

	nodes := []string{}
	for _, stage := range plan.Stages {
		nodes = append(nodes, stage.Stage)
	}

	wantNodes := []string{"planner", "code-writer", "tester", "documenter"}
	if !equalStrings(nodes, wantNodes) {
		t.Fatalf("nodes mismatch: got %#v want %#v", nodes, wantNodes)
	}

	if !hasEdge(plan.Graph.Edges, "planner", "code-writer") {
		t.Fatalf("missing planner -> code-writer edge: %#v", plan.Graph.Edges)
	}
	if !hasEdge(plan.Graph.Edges, "code-writer", "tester") {
		t.Fatalf("missing code-writer -> tester edge: %#v", plan.Graph.Edges)
	}
	if !hasEdge(plan.Graph.Edges, "tester", "documenter") {
		t.Fatalf("missing tester -> documenter edge: %#v", plan.Graph.Edges)
	}
}

func TestCodeWriterAutomaticallyAddsTester(t *testing.T) {
	plan := NewOrchestrator("").ParseRequest("간단한 앱 구현")

	if !containsStage(plan.Stages, "tester") {
		t.Fatalf("expected tester stage to be added: %#v", plan.Stages)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasEdge(edges []Edge, from, to string) bool {
	for _, edge := range edges {
		if edge.From == from && edge.To == to {
			return true
		}
	}
	return false
}

func containsStage(stages []StageDef, name string) bool {
	for _, stage := range stages {
		if stage.Stage == name {
			return true
		}
	}
	return false
}
