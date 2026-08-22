package domain

import "sort"

type TimelineItem struct {
	Sequence int64  `json:"sequence"`
	Type     string `json:"type"`
	Actor    string `json:"actor"`
	At       string `json:"at"`
}
type PackageProjection struct {
	Package      *SubtitlePackage `json:"package"`
	OpenFindings int              `json:"openFindings"`
	Timeline     []TimelineItem   `json:"timeline"`
}

func BuildProjection(p *SubtitlePackage, events []AuditEvent) PackageProjection {
	items := make([]TimelineItem, 0, len(events))
	for _, e := range events {
		items = append(items, TimelineItem{e.Sequence, e.Type, e.Actor, e.At.Format("2006-01-02 15:04:05")})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
	return PackageProjection{Package: p, OpenFindings: len(Unresolved(p)), Timeline: items}
}
