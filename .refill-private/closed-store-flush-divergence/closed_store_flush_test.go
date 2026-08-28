package closed_store_flush_divergence_test

import (
	"strings"
	"testing"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/store"
	"subtitleqc/internal/workflow"
)

func TestClosedStoreRejectsCommitWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	app := workflow.New(s)
	p, err := app.Create(workflow.CreateRequest{
		ProgramTitle:     "关闭期节目",
		EpisodeCode:      "CLOSE-01",
		AudioDurationMs:  60_000,
		Language:         "zh-CN",
		DeliveryStandard: "WebVTT",
	}, "editor-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = app.Prepare(p.ID, workflow.CueRequest{
		ExpectedVersion: p.Version,
		IdempotencyKey:  "closed-prepare",
		Cues: []domain.CaptionCue{{
			Sequence:  1,
			StartMs:   0,
			EndMs:     2_000,
			Speaker:   "播音员",
			Text:      "关闭后不得继续提交",
			SoundHint: "[提示音]",
		}},
	}, "editor-a")
	if err == nil || !strings.Contains(err.Error(), "存储已关闭") {
		t.Errorf("关闭后的提交应返回存储生命周期错误，实际 err=%v", err)
	}

	stored, ok := s.Get(p.ID)
	if !ok {
		t.Fatal("关闭后的读取不应丢失既有字幕包")
	}
	if stored.Status != domain.StatusDraft || stored.Version != p.Version || len(stored.Cues) != 0 {
		t.Errorf("失败提交污染了内存投影: status=%s version=%d cues=%d", stored.Status, stored.Version, len(stored.Cues))
	}
	if err := s.Flush(); err == nil || !strings.Contains(err.Error(), "存储已关闭") {
		t.Errorf("关闭后的 Flush 应拒绝持久化，实际 err=%v", err)
	}

	reopened, err := store.Open(dir)
	if err != nil {
		t.Fatalf("重新打开 store: %v", err)
	}
	defer reopened.Close()
	recovered, ok := reopened.Get(p.ID)
	if !ok {
		t.Fatal("重启后应保留原有字幕包")
	}
	if recovered.Status != domain.StatusDraft || recovered.Version != p.Version || len(recovered.Cues) != 0 {
		t.Errorf("失败请求被 Flush 持久化到重启投影: status=%s version=%d cues=%d", recovered.Status, recovered.Version, len(recovered.Cues))
	}
	if events := reopened.Events(p.ID); len(events) != 1 || events[0].Type != domain.EventPackageCreated {
		t.Fatalf("失败请求不应改变审计日志，实际 events=%v", events)
	}
}
