package spinningwheel

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/genie-cvc/verdandi/internal/verdandi"
)

//go:embed web/*
var webFiles embed.FS

type Server struct {
	dataDir string
}

func NewServer(dataDir string) Server {
	if dataDir == "" {
		dataDir = verdandi.DefaultDataDir()
	}
	return Server{dataDir: dataDir}
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/runs/", s.handleRunEvents)
	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(webRoot)))
	return mux
}

func (s Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runs, err := verdandi.NewStoreForDataDir(s.dataDir).List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sort.SliceStable(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
	writeJSON(w, map[string]any{"runs": runs})
}

func (s Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if runID, ok := eventStreamRunID(r.URL.Path); ok {
		s.streamRunEvents(w, r, runID)
		return
	}
	runID, ok := eventRunID(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	events, err := verdandi.NewEventStoreForDataDir(s.dataDir).List(runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"events": events})
}

func (s Server) streamRunEvents(w http.ResponseWriter, r *http.Request, runID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	events, err := verdandi.NewEventStoreForDataDir(s.dataDir).List(runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()
	for _, event := range events {
		if err := writeSSE(w, event); err != nil {
			return
		}
		flusher.Flush()
	}
	if r.URL.Query().Get("follow") != "1" {
		return
	}

	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	seen := len(events)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			latest, err := verdandi.NewEventStoreForDataDir(s.dataDir).List(runID)
			if err != nil {
				return
			}
			for _, event := range latest[seen:] {
				if err := writeSSE(w, event); err != nil {
					return
				}
				flusher.Flush()
			}
			seen = len(latest)
		}
	}
}

func eventRunID(path string) (string, bool) {
	const prefix = "/api/runs/"
	const suffix = "/events"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	runID = strings.Trim(runID, "/")
	if runID == "" {
		return "", false
	}
	return runID, true
}

func eventStreamRunID(path string) (string, bool) {
	const prefix = "/api/runs/"
	const suffix = "/events/stream"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	runID = strings.Trim(runID, "/")
	if runID == "" {
		return "", false
	}
	return runID, true
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func writeSSE(w http.ResponseWriter, event verdandi.VisualizationEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("event: visualization-event\n")); err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n\n"))
	return err
}
