package domain

import (
	"fmt"
	"sort"
)

func RunQualityChecks(p *SubtitlePackage) []ReviewFinding {
	p.NormalizeCues()
	out := make([]ReviewFinding, 0)
	for i, c := range p.Cues {
		add := func(code, sev, msg string) {
			out = append(out, ReviewFinding{ID: fmt.Sprintf("finding-%s-%d-r%d", code, i+1, len(p.Revisions)+1), PackageID: p.ID, CueID: c.ID, StartMs: c.StartMs, EndMs: c.EndMs, Source: "automatic", RuleCode: code, Severity: sev, Message: msg, Disposition: "open", ReviewRound: len(p.Revisions) + 1})
		}
		if c.StartMs < 0 || c.EndMs > p.AudioDurationMs {
			add("TIME_OUT_OF_BOUNDS", "error", "时间码超出音频范围")
		}
		if c.EndMs <= c.StartMs {
			add("TIME_INVALID", "error", "结束时间必须晚于开始时间")
		}
		if i > 0 && c.StartMs < p.Cues[i-1].EndMs {
			add("TIME_OVERLAP", "error", "时间码与上一条字幕重叠")
		}
		if c.Text == "" {
			add("EMPTY_TEXT", "error", "字幕文本不能为空")
		}
		if c.Speaker == "" {
			add("SPEAKER_MISSING", "warning", "缺少说话人")
		}
		if c.EndMs > c.StartMs && len([]rune(c.Text))*1000/(int(c.EndMs-c.StartMs)) > 20 {
			add("READING_SPEED", "warning", "阅读速度超过每秒20字")
		}
		if c.SoundHint != "" && (c.SoundHint[0] != '[' || c.SoundHint[len(c.SoundHint)-1] != ']') {
			add("SOUND_HINT_FORMAT", "warning", "声音提示应使用方括号格式")
		}
	}
	sequence := map[string]int{}
	for _, c := range p.Cues {
		sequence[c.ID] = c.Sequence
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := sequence[out[i].CueID], sequence[out[j].CueID]
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func Unresolved(p *SubtitlePackage) []ReviewFinding {
	var x []ReviewFinding
	for _, f := range p.Findings {
		if f.Disposition == "open" || f.Disposition == "question" {
			x = append(x, f)
		}
	}
	return x
}
