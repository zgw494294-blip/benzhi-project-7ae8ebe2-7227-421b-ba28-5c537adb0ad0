package workflow

import (
	"os"
	"subtitleqc/internal/domain"
	"subtitleqc/internal/store"
	"testing"
)

func TestFullFlow(t *testing.T) {
	dir := t.TempDir()
	st, e := store.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	s := New(st)
	p, e := s.Create(CreateRequest{"节目", "EP1", 60000, "zh-CN", "WebVTT"}, "tester")
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.Prepare(p.ID, CueRequest{ExpectedVersion: p.Version, Cues: []domain.CaptionCue{{Sequence: 1, StartMs: 0, EndMs: 2000, Speaker: "A", Text: "你好", SoundHint: "[音乐]"}}}, "tester")
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.Check(p.ID, p.Version, "k1", "tester")
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.SubmitReview(p.ID, p.Version, "k2", "tester")
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.Freeze(p.ID, p.Version, "k3", "tester")
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.Deliver(p.ID, p.Version, "k4", "tester")
	if e != nil || p.Status != domain.StatusDelivered {
		t.Fatalf("%v %v", e, p.Status)
	}
	_ = os.Getenv("IGNORED")
}

func TestPreviewBatchAndIdempotency(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(st)
	p, err := s.Create(CreateRequest{"节目", "EP2", 5000, "zh-CN", "WebVTT"}, "editor")
	if err != nil {
		t.Fatal(err)
	}
	before := len(st.Events(p.ID))
	if _, err = s.PreviewImport(p.ID, "0|1000|A|正常|[音乐]\n坏行"); err == nil {
		t.Fatal("预览应失败")
	}
	if len(st.Events(p.ID)) != before {
		t.Fatal("失败预览写入了事件")
	}
	p, err = s.Prepare(p.ID, CueRequest{ExpectedVersion: p.Version, IdempotencyKey: "prepare", Cues: []domain.CaptionCue{{ID: "c1", Sequence: 1, StartMs: 0, EndMs: 100, Text: "很长很长很长很长很长", Speaker: ""}, {ID: "c2", Sequence: 2, StartMs: 50, EndMs: 200, Text: "", Speaker: "B"}}}, "editor")
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.Check(p.ID, p.Version, "check", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Findings) < 2 {
		t.Fatalf("findings=%+v", p.Findings)
	}
	items := []FindingDisposition{{FindingID: p.Findings[0].ID, Disposition: "resolved"}, {FindingID: "missing", Disposition: "resolved"}}
	v, events := p.Version, len(st.Events(p.ID))
	if _, err = s.BatchDisposition(p.ID, v, "bad", "reviewer", "reviewer", items); err == nil {
		t.Fatal("整批应失败")
	}
	current, _ := st.Get(p.ID)
	if current.Version != v || len(st.Events(p.ID)) != events || current.Findings[0].Disposition != "open" {
		t.Fatal("失败批次改变了状态")
	}
	items = []FindingDisposition{{FindingID: p.Findings[0].ID, Disposition: "resolved"}, {FindingID: p.Findings[1].ID, Disposition: "resolved"}}
	p, err = s.BatchDisposition(p.ID, v, "batch", "reviewer", "reviewer", items)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Events(p.ID)) != events+2 {
		t.Fatal("每条处置应有独立审计事件")
	}
	prior := len(st.Events(p.ID))
	again, err := s.BatchDisposition(p.ID, v, "batch", "reviewer", "reviewer", items)
	if err != nil || again.Version != p.Version || len(st.Events(p.ID)) != prior {
		t.Fatal("批量处置幂等失败")
	}
}
