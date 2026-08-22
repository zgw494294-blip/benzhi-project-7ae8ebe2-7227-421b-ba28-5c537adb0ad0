package workflow

import (
	"fmt"
	"subtitleqc/internal/domain"
	"subtitleqc/internal/store"
)

type WorkbenchList struct {
	Packages []*domain.SubtitlePackage `json:"packages"`
	Counts   map[domain.Status]int     `json:"counts"`
	Metrics  store.Metrics             `json:"metrics"`
	Limit    int                       `json:"limit"`
}

func (s *Service) QueryPackages(q store.PackageQuery) WorkbenchList {
	return WorkbenchList{Packages: s.Store.Query(q), Counts: s.Store.PackageCountByStatus(), Metrics: s.Store.Metrics(), Limit: q.Limit}
}

func (s *Service) Projection(id string) (domain.PackageProjection, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return domain.PackageProjection{}, fmt.Errorf("字幕包不存在")
	}
	return domain.BuildProjection(p, s.Store.Events(id)), nil
}
func (s *Service) Audit(id string) []domain.AuditEvent { return s.Store.Events(id) }
func (s *Service) Credentials() ([]domain.DeliveryCredential, error) {
	var out []domain.DeliveryCredential
	for _, p := range s.Store.List() {
		if p.Credential != nil {
			if p.Master == nil || p.Credential.MasterChecksum != p.Master.Checksum || domain.ContentChecksum(p.Master.Content) != p.Master.Checksum {
				return nil, fmt.Errorf("字幕包%s凭据完整性校验失败", p.ID)
			}
			out = append(out, *p.Credential)
		}
	}
	return out, nil
}
func (s *Service) OpenFindings(id string) ([]domain.ReviewFinding, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("字幕包不存在")
	}
	return domain.Unresolved(p), nil
}
