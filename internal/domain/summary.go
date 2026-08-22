package domain

import "fmt"

func StatusLabel(s Status) string {
	switch s {
	case StatusDraft:
		return "草稿"
	case StatusPrepared:
		return "已准备"
	case StatusReviewing:
		return "审校中"
	case StatusCorrectionRequired:
		return "需要修订"
	case StatusReviewPassed:
		return "复审通过"
	case StatusFrozen:
		return "已冻结"
	case StatusDelivered:
		return "已交付"
	default:
		return string(s)
	}
}
func FindingSummary(p *SubtitlePackage) string {
	open := len(Unresolved(p))
	return fmt.Sprintf("共%d条字幕，未解决发现%d项", len(p.Cues), open)
}
