package workflow

import (
	"fmt"
	"subtitleqc/internal/domain"
)

type WorkbenchReport struct {
	Package   domain.PackageProjection `json:"package"`
	Checklist domain.Checklist         `json:"checklist"`
	Summary   string                   `json:"summary"`
}

func (s *Service) Report(id string) (WorkbenchReport, error) {
	s.reportMu.Lock()
	report, cached := s.reportMap[id]
	s.reportMu.Unlock()
	if cached {
		return report, nil
	}
	proj, e := s.Projection(id)
	if e != nil {
		return WorkbenchReport{}, e
	}
	report = WorkbenchReport{Package: proj, Checklist: domain.BuildChecklist(proj.Package), Summary: domain.FindingSummary(proj.Package)}
	s.reportMu.Lock()
	s.reportMap[id] = report
	s.reportMu.Unlock()
	return report, nil
}
func (s *Service) EnsureReady(id string) error {
	p, ok := s.Store.Get(id)
	if !ok {
		return fmt.Errorf("字幕包不存在")
	}
	if len(p.Cues) == 0 {
		return fmt.Errorf("没有字幕条目")
	}
	return nil
}
