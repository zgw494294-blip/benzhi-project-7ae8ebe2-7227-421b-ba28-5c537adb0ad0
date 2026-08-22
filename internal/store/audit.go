package store

import (
	"sort"
	"subtitleqc/internal/domain"
	"time"
)

type AuditQuery struct {
	PackageID string
	Type      string
	Actor     string
	From      time.Time
	To        time.Time
	Limit     int
}

func FilterEvents(events []domain.AuditEvent, q AuditQuery) []domain.AuditEvent {
	out := make([]domain.AuditEvent, 0)
	for _, e := range events {
		if q.PackageID != "" && e.PackageID != q.PackageID {
			continue
		}
		if q.Type != "" && e.Type != q.Type {
			continue
		}
		if q.Actor != "" && e.Actor != q.Actor {
			continue
		}
		if !q.From.IsZero() && e.At.Before(q.From) {
			continue
		}
		if !q.To.IsZero() && e.At.After(q.To) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out
}
func EventTypes(events []domain.AuditEvent) map[string]int {
	m := map[string]int{}
	for _, e := range events {
		m[e.Type]++
	}
	return m
}
