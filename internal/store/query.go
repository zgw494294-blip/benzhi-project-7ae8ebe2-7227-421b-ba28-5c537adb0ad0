package store

import (
	"sort"
	"strings"
	"subtitleqc/internal/domain"
)

type PackageQuery struct {
	Status   domain.Status
	Text     string
	RuleCode string
	Limit    int
	LimitSet bool
}

const MaxPackageLimit = 100
const DefaultPackageLimit = 50

func (s *Store) Query(q PackageQuery) []*domain.SubtitlePackage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached, ok := s.queryCache[q]; ok {
		return clonePackageSlice(cached)
	}
	out := make([]*domain.SubtitlePackage, 0)
	needle := strings.ToLower(strings.TrimSpace(q.Text))
	for _, p := range s.snapshot.Packages {
		if q.Status != "" && p.Status != q.Status {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(p.ProgramTitle), needle) && !strings.Contains(strings.ToLower(p.EpisodeCode), needle) {
			continue
		}
		if q.RuleCode != "" {
			matched := false
			for _, finding := range p.Findings {
				if finding.RuleCode == q.RuleCode {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, clonePackage(p))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if q.LimitSet && q.Limit == 0 {
		out = []*domain.SubtitlePackage{}
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	s.queryCache[q] = out
	return clonePackageSlice(out)
}

func clonePackageSlice(in []*domain.SubtitlePackage) []*domain.SubtitlePackage {
	out := make([]*domain.SubtitlePackage, len(in))
	for i, p := range in {
		out[i] = clonePackage(p)
	}
	return out
}

func (s *Store) PackageCountByStatus() map[domain.Status]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := map[domain.Status]int{}
	for _, p := range s.snapshot.Packages {
		m[p.Status]++
	}
	return m
}

func NormalizePackageLimit(limit int) int {
	if limit <= 0 {
		return limit
	}
	if limit > MaxPackageLimit {
		return MaxPackageLimit
	}
	return limit
}
