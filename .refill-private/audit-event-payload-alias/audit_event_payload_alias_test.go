package auditeventpayloadalias_test

import (
	"testing"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/store"
)

func TestAuditEventsDoNotExposeMutablePayload(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	p := &domain.SubtitlePackage{ID: "pkg-audit-alias", Status: domain.StatusDraft, Version: 1}
	payload := map[string]any{
		"status": "draft",
		"details": map[string]any{
			"labels": []any{"原始标签"},
		},
	}
	if err := s.Commit(p.ID, "", 0, p, domain.EventPackageCreated, "editor", payload); err != nil {
		t.Fatal(err)
	}

	first := s.Events(p.ID)
	if len(first) != 1 {
		t.Fatalf("expected one audit event, got %d", len(first))
	}
	first[0].Payload["details"].(map[string]any)["labels"].([]any)[0] = "篡改标签"

	second := s.Events(p.ID)
	gotDetails := second[0].Payload["details"].(map[string]any)
	gotLabels := gotDetails["labels"].([]any)
	if second[0].Payload["status"] != "draft" || gotLabels[0] != "原始标签" {
		t.Fatalf("audit payload mutation leaked into later reads: %#v", second[0].Payload)
	}
}
