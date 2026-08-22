package store

import (
	"encoding/json"
	"fmt"
	"subtitleqc/internal/domain"
)

func encodeEvent(e domain.AuditEvent) ([]byte, error) { return json.Marshal(e) }
func decodeEvent(b []byte) (domain.AuditEvent, error) {
	var e domain.AuditEvent
	if err := json.Unmarshal(b, &e); err != nil {
		return e, fmt.Errorf("事件解码失败: %w", err)
	}
	if e.Sequence < 1 || e.Type == "" {
		return e, fmt.Errorf("事件字段缺失")
	}
	return e, nil
}
func encodeSnapshot(s Snapshot) ([]byte, error) { return json.MarshalIndent(s, "", "  ") }
func decodeSnapshot(b []byte) (Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	if s.Packages == nil {
		s.Packages = map[string]*domain.SubtitlePackage{}
	}
	return s, nil
}
