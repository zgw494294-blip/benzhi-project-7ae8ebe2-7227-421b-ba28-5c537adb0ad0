package domain

import "testing"

func TestTransitionAndChecks(t *testing.T) {
	p := &SubtitlePackage{ID: "p", ProgramTitle: "节目", EpisodeCode: "E1", AudioDurationMs: 5000, Language: "zh", DeliveryStandard: "vtt", Status: StatusDraft, Version: 1, Cues: []CaptionCue{{ID: "c", Sequence: 1, StartMs: 100, EndMs: 1000, Text: "文本"}}}
	if err := Transition(p, StatusPrepared); err != nil {
		t.Fatal(err)
	}
	p.Findings = RunQualityChecks(p)
	if len(p.Findings) != 1 || p.Findings[0].RuleCode != "SPEAKER_MISSING" {
		t.Fatalf("findings=%+v", p.Findings)
	}
	if _, _, sum := WebVTT(p); len(sum) != 64 {
		t.Fatal("checksum")
	}
}

func TestParseCueLinesAndChecklist(t *testing.T) {
	if _, err := ParseCueLines("p", "0|1000|A|ok|[音乐]\n坏行"); err == nil || err.Error() != "第2行字段不足" {
		t.Fatalf("err=%v", err)
	}
	cues, err := ParseCueLines("p", "1000|2000|A|后|[音乐]\n0|900|B|前|[提示]")
	if err != nil {
		t.Fatal(err)
	}
	p := &SubtitlePackage{ID: "p", AudioDurationMs: 3000, Cues: cues, Findings: []ReviewFinding{{RuleCode: "X", Severity: "error", Disposition: "open"}, {RuleCode: "Y", Severity: "warning", Disposition: "question"}}}
	p.NormalizeCues()
	c := BuildChecklist(p)
	if len(cues) != 2 || cues[0].Revision != 1 || !c.Blocking() || c.Open != 1 || c.Question != 1 {
		t.Fatalf("cues=%+v checklist=%+v", cues, c)
	}
}
