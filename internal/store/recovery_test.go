package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"subtitleqc/internal/domain"
)

func TestOpenRejectsBrokenEventChecksum(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &domain.SubtitlePackage{ID: "p1", ProgramTitle: "节目", EpisodeCode: "EP1", AudioDurationMs: 1000, Language: "zh-CN", DeliveryStandard: "WebVTT", Status: domain.StatusDraft, Version: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.Commit(p.ID, "", 0, p, domain.EventPackageCreated, "tester", map[string]any{"status": "draft"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "events.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var event domain.AuditEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	event.Checksum = "broken"
	raw, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("损坏校验和应阻止恢复")
	}
}
