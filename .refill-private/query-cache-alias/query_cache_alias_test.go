package querycachealias_test

import (
	"testing"
	"time"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/store"
)

func TestQueryCacheReturnsImmutablePackages(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().UTC()
	p := &domain.SubtitlePackage{
		ID: "pkg-cache", ProgramTitle: "缓存边界测试", EpisodeCode: "CACHE-1",
		AudioDurationMs: 5000, Language: "zh-CN", DeliveryStandard: "WebVTT",
		Status: domain.StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now,
		Cues: []domain.CaptionCue{{ID: "cue-1", PackageID: "pkg-cache", Sequence: 1, StartMs: 0, EndMs: 1000, Speaker: "主持人", Text: "原始字幕", Revision: 1}},
	}
	if err := st.Commit(p.ID, "create-cache", 0, p, domain.EventPackageCreated, "tester", map[string]any{"status": domain.StatusDraft}); err != nil {
		t.Fatal(err)
	}

	query := store.PackageQuery{Text: "缓存边界"}
	first := st.Query(query)
	if len(first) != 1 || len(first[0].Cues) != 1 {
		t.Fatalf("首次查询结果异常: %+v", first)
	}
	first[0].Cues[0].Text = "调用方污染"

	again := st.Query(query)
	if got := again[0].Cues[0].Text; got != "原始字幕" {
		t.Fatalf("查询缓存泄漏了调用方修改: got %q, want %q", got, "原始字幕")
	}
}
