package store

import (
	"os"
	"path/filepath"
	"time"
)

func (s *Store) DataDir() string      { return s.dir }
func (s *Store) SnapshotPath() string { return filepath.Join(s.dir, "snapshot.json") }
func (s *Store) EventsPath() string   { return filepath.Join(s.dir, "events.jsonl") }
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eventFile != nil {
		if err := s.eventFile.Sync(); err != nil {
			return err
		}
	}
	return s.writeSnapshot()
}
func (s *Store) LastUpdated() time.Time {
	if st, err := os.Stat(s.SnapshotPath()); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}
