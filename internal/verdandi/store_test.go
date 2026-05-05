package verdandi

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStoreConcurrentSavesPreserveRecords(t *testing.T) {
	dataDir := t.TempDir()
	store := NewStoreForDataDir(dataDir)
	start := make(chan struct{})
	var wg sync.WaitGroup

	const count = 32
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			err := store.Save(RunRecord{
				RunID:       fmt.Sprintf("run_%02d", index),
				Status:      "success",
				Request:     "concurrent save",
				Summary:     Summary{TotalStages: 1, Success: 1, Files: []FileEntry{}},
				CreatedAt:   time.Unix(int64(index), 0).UTC(),
				CompletedAt: time.Unix(int64(index), 0).UTC(),
			})
			if err != nil {
				t.Errorf("save %d failed: %v", index, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	runs, err := store.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(runs) != count {
		t.Fatalf("expected %d records after concurrent saves, got %d: %#v", count, len(runs), runs)
	}
}

func TestEventStoreListsLargeEventLines(t *testing.T) {
	dataDir := t.TempDir()
	store := NewEventStoreForDataDir(dataDir)
	runID := "run_large_event"
	message := strings.Repeat("긴 이벤트 메시지 ", 12000)

	if err := store.Save(runID, []VisualizationEvent{{
		RunID:     runID,
		Type:      EventRunStarted,
		Timestamp: time.Unix(1, 0).UTC(),
		Message:   message,
	}}); err != nil {
		t.Fatalf("save event: %v", err)
	}

	events, err := store.List(runID)
	if err != nil {
		t.Fatalf("list large event: %v", err)
	}
	if len(events) != 1 || events[0].Message != message {
		t.Fatalf("large event was not preserved: %#v", events)
	}
}
