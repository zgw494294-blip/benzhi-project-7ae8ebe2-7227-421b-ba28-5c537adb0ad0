package workflow

import (
	"testing"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/store"
)

func newReviewingPackage(t *testing.T, s *Service, episode string, cues []domain.CaptionCue) *domain.SubtitlePackage {
	t.Helper()
	p, err := s.Create(CreateRequest{ProgramTitle: "测试节目", EpisodeCode: episode, AudioDurationMs: 10000, Language: "zh-CN", DeliveryStandard: "WebVTT WCAG"}, "editor")
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.Prepare(p.ID, CueRequest{ExpectedVersion: p.Version, IdempotencyKey: "prepare-" + episode, Cues: cues}, "editor")
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.Check(p.ID, p.Version, "check-"+episode, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestManualFindingValidationAndIdempotency(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(st)
	p := newReviewingPackage(t, s, "M01", []domain.CaptionCue{{ID: "c1", Sequence: 1, StartMs: 0, EndMs: 1000, Speaker: "主持人", Text: "欢迎收听"}})

	req := ManualFindingRequest{ExpectedVersion: p.Version, IdempotencyKey: "manual-1", Role: "reviewer", CueID: "c1", RuleCode: "WORDING", Severity: "warning", Message: "措辞需要更清晰"}
	p, err = s.AddManualFinding(p.ID, req, "reviewer-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Findings) != 1 || p.Findings[0].Source != "manual" || p.Findings[0].Disposition != "open" || p.Findings[0].CreatedBy != "reviewer-a" {
		t.Fatalf("人工发现未进入统一队列: %+v", p.Findings)
	}
	eventCount := len(st.Events(p.ID))
	again, err := s.AddManualFinding(p.ID, req, "reviewer-a")
	if err != nil || again.Version != p.Version || len(st.Events(p.ID)) != eventCount {
		t.Fatal("人工发现幂等重放产生了写入")
	}

	bad := req
	bad.ExpectedVersion, bad.IdempotencyKey, bad.CueID = p.Version, "manual-bad", "missing"
	if _, err = s.AddManualFinding(p.ID, bad, "reviewer-a"); err == nil {
		t.Fatal("不存在的 cueId 应被拒绝")
	}
	duplicate := req
	duplicate.ExpectedVersion, duplicate.IdempotencyKey = p.Version, "manual-duplicate"
	if _, err = s.AddManualFinding(p.ID, duplicate, "reviewer-a"); err == nil {
		t.Fatal("同轮次重复人工发现应被拒绝")
	}
	current, _ := st.Get(p.ID)
	if current.Version != p.Version || len(current.Findings) != 1 || len(st.Events(p.ID)) != eventCount {
		t.Fatal("失败登记改变了投影或审计日志")
	}

	p, err = s.SubmitReview(p.ID, p.Version, "review-manual", "reviewer-a")
	if err != nil || p.Status != domain.StatusCorrectionRequired {
		t.Fatalf("人工发现未阻断复审: %v %s", err, p.Status)
	}
}

func TestRevisionPreviewAndPreciseFindingLinks(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(st)
	p := newReviewingPackage(t, s, "R01", []domain.CaptionCue{
		{ID: "c1", Sequence: 1, StartMs: 0, EndMs: 1000, Speaker: "甲", Text: "第一条"},
		{ID: "c2", Sequence: 2, StartMs: 1200, EndMs: 2200, Speaker: "乙", Text: "第二条"},
	})
	for i, cueID := range []string{"c1", "c2"} {
		p, err = s.AddManualFinding(p.ID, ManualFindingRequest{ExpectedVersion: p.Version, IdempotencyKey: "manual-r" + string(rune('1'+i)), Role: "reviewer", CueID: cueID, RuleCode: "WORDING", Severity: "warning", Message: "问题" + cueID}, "reviewer")
		if err != nil {
			t.Fatal(err)
		}
	}
	p, err = s.SubmitReview(p.ID, p.Version, "review-r", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	before := p.Cues[0]
	after := before
	after.Text = "第一条已修订"
	change := domain.RevisionChange{CueID: before.ID, Before: before, After: after}
	impact, err := s.PreviewRevision(p.ID, RevisionPreviewRequest{Changes: []domain.RevisionChange{change}, FindingIDs: []string{p.Findings[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(impact.Diffs) != 1 || len(impact.LinkableFindings) != 1 || len(impact.Uncovered) != 1 {
		t.Fatalf("修订影响不准确: %+v", impact)
	}
	if _, err = s.PreviewRevision(p.ID, RevisionPreviewRequest{Changes: []domain.RevisionChange{change}, FindingIDs: []string{p.Findings[1].ID}}); err == nil {
		t.Fatal("其他 cue 的 findingId 应被拒绝")
	}

	p, err = s.Revise(p.ID, p.Version, "revision-r", "调整措辞", "editor", []domain.RevisionChange{change}, []string{p.Findings[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Revisions) != 1 || len(p.Revisions[0].FindingIDs) != 1 || len(p.Findings) != 2 || p.Findings[0].Disposition != "resolved" || p.Findings[1].Disposition != "open" {
		t.Fatalf("修订没有精确保留发现: %+v", p)
	}
	if _, err = s.SubmitReview(p.ID, p.Version, "too-early", "reviewer"); err == nil {
		t.Fatal("修订后未质检不应提交复审")
	}
	p, err = s.Check(p.ID, p.Version, "recheck-r", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.SubmitReview(p.ID, p.Version, "review-r2", "reviewer")
	if err != nil || p.Status != domain.StatusCorrectionRequired {
		t.Fatalf("未关联发现应继续阻断复审: %v %s", err, p.Status)
	}
}

func TestFreezePreviewChecksumConfirmation(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(st)
	p := newReviewingPackage(t, s, "F01", []domain.CaptionCue{{ID: "c1", Sequence: 1, StartMs: 0, EndMs: 1000, Speaker: "主持人", Text: "确定内容"}})
	p, err = s.SubmitReview(p.ID, p.Version, "review-f", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.PreviewFreeze(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.PreviewFreeze(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("相同投影预检结果不确定: %+v %+v", first, second)
	}
	version, events := p.Version, len(st.Events(p.ID))
	if _, err = s.FreezeConfirmed(p.ID, p.Version, "bad-checksum", "freeze-bad", "delivery"); err == nil {
		t.Fatal("旧校验和应拒绝冻结")
	}
	current, _ := st.Get(p.ID)
	if current.Version != version || current.Master != nil || len(st.Events(p.ID)) != events {
		t.Fatal("失败冻结改变了状态")
	}
	p, err = s.FreezeConfirmed(p.ID, first.ExpectedVersion, first.Checksum, "freeze-ok", "delivery")
	if err != nil || p.Status != domain.StatusFrozen || p.Master == nil {
		t.Fatalf("确认冻结失败: %v", err)
	}
}

func TestQualityStatisticsEmptyAndStable(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(st)
	empty := s.QualityStatistics(StatisticsFilter{Language: "not-found"})
	if empty.PackageCount != 0 || empty.DeliveredCount != 0 || empty.Findings == nil {
		t.Fatalf("空统计结构不完整: %+v", empty)
	}
	p := newReviewingPackage(t, s, "S01", []domain.CaptionCue{{ID: "c1", Sequence: 1, StartMs: 0, EndMs: 100, Text: "很长很长很长很长很长"}})
	if len(p.Findings) < 2 {
		t.Fatalf("测试发现不足: %+v", p.Findings)
	}
	first := s.QualityStatistics(StatisticsFilter{Language: "zh-CN"})
	second := s.QualityStatistics(StatisticsFilter{Language: "zh-CN"})
	if len(first.Findings) != len(second.Findings) {
		t.Fatal("重复统计结果不一致")
	}
	for i := range first.Findings {
		if first.Findings[i] != second.Findings[i] {
			t.Fatal("统计排序不稳定")
		}
		if i > 0 && first.Findings[i-1].RuleCode > first.Findings[i].RuleCode {
			t.Fatal("ruleCode 未稳定排序")
		}
	}
}
