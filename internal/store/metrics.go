package store

import "subtitleqc/internal/domain"

type Metrics struct {
	Packages  int                   `json:"packages"`
	Events    int                   `json:"events"`
	Delivered int                   `json:"delivered"`
	Findings  int                   `json:"findings"`
	ByStatus  map[domain.Status]int `json:"byStatus"`
}

func (s *Store) Metrics() Metrics {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := Metrics{Packages: len(s.snapshot.Packages), Events: len(s.events), ByStatus: map[domain.Status]int{}}
	for _, p := range s.snapshot.Packages {
		m.ByStatus[p.Status]++
		if p.Status == domain.StatusDelivered {
			m.Delivered++
		}
		m.Findings += len(p.Findings)
	}
	return m
}
