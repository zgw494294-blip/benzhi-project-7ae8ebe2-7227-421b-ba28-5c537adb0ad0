package domain

import (
	"fmt"
	"regexp"
	"strings"
)

var episodePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)

type TimeRange struct {
	StartMs int64
	EndMs   int64
}

func NewTimeRange(start, end int64) (TimeRange, error) {
	if start < 0 || end <= start {
		return TimeRange{}, fmt.Errorf("时间范围无效")
	}
	return TimeRange{start, end}, nil
}
func (t TimeRange) Duration() int64        { return t.EndMs - t.StartMs }
func (t TimeRange) Contains(ms int64) bool { return ms >= t.StartMs && ms <= t.EndMs }
func NormalizeLanguage(v string) string    { return strings.ToLower(strings.TrimSpace(v)) }
func ValidateEpisodeCode(v string) error {
	if !episodePattern.MatchString(strings.TrimSpace(v)) {
		return fmt.Errorf("期号格式无效")
	}
	return nil
}
func ValidateStandard(v string) error {
	v = strings.TrimSpace(v)
	if len(v) < 3 {
		return fmt.Errorf("交付标准过短")
	}
	return nil
}
func ValidateSpeaker(v string) error {
	if len([]rune(strings.TrimSpace(v))) > 80 {
		return fmt.Errorf("说话人过长")
	}
	return nil
}

func ValidateText(v string) error {
	if len([]rune(strings.TrimSpace(v))) > 20000 {
		return fmt.Errorf("字幕文本过长")
	}
	return nil
}
func ValidateSoundHint(v string) error {
	if v == "" {
		return nil
	}
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return fmt.Errorf("声音提示格式无效")
	}
	return nil
}
