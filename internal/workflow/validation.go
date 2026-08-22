package workflow

import (
	"fmt"
	"subtitleqc/internal/domain"
)

func ValidateCueSet(p *domain.SubtitlePackage) error {
	if len(p.Cues) == 0 {
		return fmt.Errorf("字幕条目不能为空")
	}
	p.NormalizeCues()
	for i, c := range p.Cues {
		if _, err := domain.NewTimeRange(c.StartMs, c.EndMs); err != nil {
			return fmt.Errorf("第%d条: %w", i+1, err)
		}
		if c.EndMs > p.AudioDurationMs {
			return fmt.Errorf("第%d条超出音频时长", i+1)
		}
		if err := domain.ValidateSpeaker(c.Speaker); err != nil {
			return fmt.Errorf("第%d条: %w", i+1, err)
		}
		if err := domain.ValidateSoundHint(c.SoundHint); err != nil {
			return fmt.Errorf("第%d条: %w", i+1, err)
		}
		if err := domain.ValidateText(c.Text); err != nil {
			return fmt.Errorf("第%d条: %w", i+1, err)
		}
	}
	return nil
}
func HasBlockingFindings(p *domain.SubtitlePackage) bool {
	for _, f := range p.Findings {
		if f.Disposition == "open" && f.Severity == "error" {
			return true
		}
	}
	return false
}
