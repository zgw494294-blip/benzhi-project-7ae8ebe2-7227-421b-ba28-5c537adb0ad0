package workflow

import (
	"sort"
	"subtitleqc/internal/domain"
	"time"
)

type HistoryFilter struct {
	From  time.Time
	To    time.Time
	Types []string
}

func (s *Service) History(id string, f HistoryFilter) []domain.AuditEvent {
	events := s.Store.Events(id)
	if f.From.IsZero() && f.To.IsZero() && len(f.Types) == 0 {
		return events
	}
	allowed := map[string]bool{}
	for _, t := range f.Types {
		allowed[t] = true
	}
	out := make([]domain.AuditEvent, 0, len(events))
	for _, e := range events {
		if !f.From.IsZero() && e.At.Before(f.From) {
			continue
		}
		if !f.To.IsZero() && e.At.After(f.To) {
			continue
		}
		if len(allowed) > 0 && !allowed[e.Type] {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}
func (s *Service) LatestEvent(id string) (domain.AuditEvent, bool) {
	events := s.Store.Events(id)
	if len(events) == 0 {
		return domain.AuditEvent{}, false
	}
	return events[len(events)-1], true
}
func EventLabel(t string) string {
	switch t {
	case domain.EventPackageCreated:
		return "创建字幕包"
	case domain.EventCuesPrepared:
		return "准备字幕条目"
	case domain.EventQualityChecked:
		return "自动质检"
	case domain.EventManualFindingAdded:
		return "登记人工发现"
	case domain.EventFindingDispositioned:
		return "处置质检发现"
	case domain.EventReviewSubmitted:
		return "提交审校"
	case domain.EventRevisionCreated:
		return "创建修订批次"
	case domain.EventMasterFrozen:
		return "冻结母版"
	case domain.EventCredentialIssued:
		return "签发交付凭据"
	default:
		return t
	}
}
func EventTimeline(events []domain.AuditEvent) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, e := range events {
		out = append(out, map[string]any{"sequence": e.Sequence, "label": EventLabel(e.Type), "actor": e.Actor, "at": e.At})
	}
	return out
}
