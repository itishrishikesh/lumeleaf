package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}
type State struct {
	Version int        `json:"version"`
	Active  Location   `json:"active"`
	Open    []Location `json:"open"`
	Theme   string     `json:"theme"`
	SavedAt time.Time  `json:"savedAt"`
}

func Save(path string, s State) error {
	s.Version = 1
	s.SavedAt = time.Now().UTC()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, err
	}
	if s.Version != 1 {
		return State{}, fmt.Errorf("unsupported session version %d", s.Version)
	}
	return s, nil
}
