package domain

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseCueLines(packageID, raw string) ([]CaptionCue, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("没有可导入的字幕行")
	}
	lines := strings.Split(raw, "\n")
	var cues []CaptionCue
	for i, line := range lines {
		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 {
			return nil, fmt.Errorf("第%d行字段不足", i+1)
		}
		start, e1 := strconv.ParseInt(parts[0], 10, 64)
		end, e2 := strconv.ParseInt(parts[1], 10, 64)
		if e1 != nil || e2 != nil {
			return nil, fmt.Errorf("第%d行时间码无效", i+1)
		}
		if end <= start || start < 0 {
			return nil, fmt.Errorf("第%d行时间范围无效", i+1)
		}
		cues = append(cues, CaptionCue{ID: fmt.Sprintf("cue-import-%d", i+1), PackageID: packageID, Sequence: i + 1, StartMs: start, EndMs: end, Speaker: strings.TrimSpace(parts[2]), Text: strings.TrimSpace(parts[3]), SoundHint: strings.TrimSpace(parts[4]), Revision: 1})
	}
	return cues, nil
}
func ExportCueLines(cues []CaptionCue) string {
	var b strings.Builder
	for _, c := range cues {
		fmt.Fprintf(&b, "%d|%d|%s|%s|%s\n", c.StartMs, c.EndMs, c.Speaker, c.Text, c.SoundHint)
	}
	return b.String()
}
