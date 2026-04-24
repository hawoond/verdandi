package verdandi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Store struct {
	path string
}

type runStore struct {
	Runs []RunRecord `json:"runs"`
}

func NewStore(path string) Store {
	return Store{path: path}
}

func (s Store) Save(record RunRecord) error {
	store, err := s.load()
	if err != nil {
		return err
	}
	store.Runs = append(store.Runs, record)
	if len(store.Runs) > 100 {
		store.Runs = store.Runs[len(store.Runs)-100:]
	}
	return s.write(store)
}

func (s Store) Find(runID string) (RunRecord, error) {
	store, err := s.load()
	if err != nil {
		return RunRecord{}, err
	}
	for _, record := range store.Runs {
		if record.RunID == runID {
			return record, nil
		}
	}
	return RunRecord{}, fmt.Errorf("runId를 찾을 수 없습니다: %s", runID)
}

func (s Store) load() (runStore, error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return runStore{}, err
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return runStore{Runs: []RunRecord{}}, nil
		}
		return runStore{}, err
	}
	if len(data) == 0 {
		return runStore{Runs: []RunRecord{}}, nil
	}

	var store runStore
	if err := json.Unmarshal(data, &store); err != nil {
		return runStore{}, err
	}
	if store.Runs == nil {
		store.Runs = []RunRecord{}
	}
	return store, nil
}

func (s Store) write(store runStore) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
