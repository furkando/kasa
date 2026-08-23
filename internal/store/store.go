package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Snapshot is one day's portfolio valuation, written on every run.
// True history accumulates here even as holdings change over time.
type Snapshot struct {
	Date     string             `json:"date"` // 2006-01-02
	TotalTRY float64            `json:"total_try"`
	TotalUSD float64            `json:"total_usd"`
	Assets   map[string]float64 `json:"assets"` // asset key -> value TRY
}

type Store struct {
	path      string
	Snapshots map[string]Snapshot `json:"snapshots"`
}

func dataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "kasa")
}

func Open() (*Store, error) {
	s := &Store{path: filepath.Join(dataDir(), "snapshots.json"), Snapshots: map[string]Snapshot{}}
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.Snapshots); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Put(snap Snapshot) error {
	if s.path == "" { // zero-value store (tests): no-op
		return nil
	}
	if s.Snapshots == nil {
		s.Snapshots = map[string]Snapshot{}
	}
	s.Snapshots[snap.Date] = snap
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.Snapshots, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Sorted returns snapshots oldest-first.
func (s *Store) Sorted() []Snapshot {
	out := make([]Snapshot, 0, len(s.Snapshots))
	for _, v := range s.Snapshots {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func Today() string { return time.Now().Format("2006-01-02") }
