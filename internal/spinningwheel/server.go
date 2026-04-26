package spinningwheel

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"sort"
	"strings"

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

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
