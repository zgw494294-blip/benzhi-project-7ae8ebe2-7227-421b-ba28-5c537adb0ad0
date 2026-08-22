package store

import (
	"fmt"
	"subtitleqc/internal/domain"
)

func (s *Store) Validate() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot.SchemaVersion != 1 {
		return fmt.Errorf("schemaVersion 无效")
	}
	if s.snapshot.Sequence != int64(len(s.events)) {
		return fmt.Errorf("快照序号与日志不一致")
	}
	for id, p := range s.snapshot.Packages {
		if id == "" || p == nil || p.ID != id {
			return fmt.Errorf("投影主键不一致")
		}
	}
	for i, e := range s.events {
		if e.Sequence != int64(i+1) || e.PackageID == "" || e.Checksum == "" {
			return fmt.Errorf("事件投影无效")
		}
		if _, ok := s.snapshot.Packages[e.PackageID]; !ok {
			return fmt.Errorf("事件引用不存在的字幕包")
		}
	}
	return nil
}
func (s *Store) ReplayEvents() []domain.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.AuditEvent, len(s.events))
	copy(out, s.events)
	return out
}
