package workflow

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"subtitleqc/internal/domain"
)

type ManualFindingRequest struct {
	ExpectedVersion int
	IdempotencyKey  string
	Role            string
	CueID           string
	StartMs         *int64
	EndMs           *int64
	RuleCode        string
	Severity        string
	Message         string
}

type RevisionPreviewRequest struct {
	Changes    []domain.RevisionChange
	FindingIDs []string
}

type StatisticsFilter struct {
	From             time.Time
	To               time.Time
	Language         string
	DeliveryStandard string
}

func (s *Service) AddManualFinding(id string, req ManualFindingRequest, actor string) (*domain.SubtitlePackage, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("字幕包不存在")
	}
	if replayed, err := s.Store.CheckIdempotency(req.IdempotencyKey, id, domain.EventManualFindingAdded); err != nil {
		return nil, err
	} else if replayed {
		return p, nil
	}
	ctx := CommandContext{Actor: actor, Role: req.Role, ExpectedVersion: req.ExpectedVersion, IdempotencyKey: req.IdempotencyKey}
	if err := ctx.Validate("finding"); err != nil {
		return nil, err
	}
	if strings.ToLower(strings.TrimSpace(req.Role)) != "reviewer" {
		return nil, fmt.Errorf("只有审校员可以登记人工发现")
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return nil, fmt.Errorf("idempotencyKey 不能为空")
	}
	if err := ensureExpected(p.Version, req.ExpectedVersion); err != nil {
		return nil, err
	}
	if p.Status != domain.StatusReviewing {
		return nil, fmt.Errorf("只有 reviewing 状态可以登记人工发现")
	}
	ruleCode := strings.TrimSpace(req.RuleCode)
	message := strings.TrimSpace(req.Message)
	if ruleCode == "" {
		return nil, fmt.Errorf("问题分类不能为空")
	}
	if req.Severity != "error" && req.Severity != "warning" {
		return nil, fmt.Errorf("severity 必须为 error 或 warning")
	}
	if message == "" {
		return nil, fmt.Errorf("问题描述不能为空")
	}
	if len([]rune(message)) > 2000 {
		return nil, fmt.Errorf("问题描述过长")
	}

	cueID := strings.TrimSpace(req.CueID)
	var startMs, endMs int64
	if cueID != "" {
		if req.StartMs != nil || req.EndMs != nil {
			return nil, fmt.Errorf("cueId 与时间范围只能选择一种定位")
		}
		found := false
		for _, cue := range p.Cues {
			if cue.ID == cueID {
				startMs, endMs, found = cue.StartMs, cue.EndMs, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("cueId 不属于当前字幕包")
		}
	} else {
		if req.StartMs == nil || req.EndMs == nil {
			return nil, fmt.Errorf("必须提供 cueId 或完整时间范围")
		}
		startMs, endMs = *req.StartMs, *req.EndMs
		if _, err := domain.NewTimeRange(startMs, endMs); err != nil {
			return nil, err
		}
		if endMs > p.AudioDurationMs {
			return nil, fmt.Errorf("时间范围超出音频时长")
		}
	}

	round := len(p.Revisions) + 1
	for _, finding := range p.Findings {
		if finding.ReviewRound == round && finding.CueID == cueID && finding.StartMs == startMs && finding.EndMs == endMs && strings.TrimSpace(finding.Message) == message {
			return nil, fmt.Errorf("同一审校轮次存在相同定位和描述的人工发现")
		}
	}
	now := time.Now().UTC()
	finding := domain.ReviewFinding{
		ID: fmt.Sprintf("finding-manual-%d", time.Now().UnixNano()), PackageID: id, CueID: cueID,
		StartMs: startMs, EndMs: endMs, Source: "manual", RuleCode: ruleCode,
		Severity: req.Severity, Message: message, Disposition: "open", ReviewRound: round,
		CreatedBy: actor, CreatedAt: &now,
	}
	p.Findings = append(p.Findings, finding)
	p.Version++
	p.UpdatedAt = now
	if err := s.Store.Commit(id, req.IdempotencyKey, req.ExpectedVersion, p, domain.EventManualFindingAdded, actor, map[string]any{
		"findingId": finding.ID, "cueId": cueID, "startMs": startMs, "endMs": endMs,
		"ruleCode": ruleCode, "severity": req.Severity, "message": message, "reviewRound": round,
	}); err != nil {
		return nil, err
	}
	stored, _ := s.Store.Get(id)
	return stored, nil
}

func (s *Service) PreviewRevision(id string, req RevisionPreviewRequest) (domain.RevisionImpact, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return domain.RevisionImpact{}, fmt.Errorf("字幕包不存在")
	}
	if p.Status != domain.StatusCorrectionRequired {
		return domain.RevisionImpact{}, fmt.Errorf("只有 correction_required 状态可以预检修订")
	}
	_, impact, err := validateRevision(p, req.Changes, req.FindingIDs)
	return impact, err
}

func validateRevision(p *domain.SubtitlePackage, changes []domain.RevisionChange, findingIDs []string) ([]domain.RevisionChange, domain.RevisionImpact, error) {
	impact := domain.RevisionImpact{Diffs: []domain.CueDiff{}, LinkableFindings: []domain.ReviewFinding{}, Uncovered: []domain.ReviewFinding{}, AdjacentCues: []domain.CaptionCue{}}
	if len(changes) == 0 {
		return nil, impact, fmt.Errorf("修订内容不能为空")
	}
	normalized := append([]domain.RevisionChange(nil), changes...)
	changed := map[string]bool{}
	timeChanged := map[string]bool{}
	projected := *p
	projected.Cues = append([]domain.CaptionCue(nil), p.Cues...)
	for i := range normalized {
		ch := &normalized[i]
		if changed[ch.CueID] {
			return nil, impact, fmt.Errorf("修订条目重复")
		}
		changed[ch.CueID] = true
		idx := -1
		for j := range p.Cues {
			if p.Cues[j].ID == ch.CueID {
				idx = j
				break
			}
		}
		if idx < 0 {
			return nil, impact, fmt.Errorf("字幕条目%s不存在", ch.CueID)
		}
		current := p.Cues[idx]
		if !reflect.DeepEqual(ch.Before, current) {
			return nil, impact, fmt.Errorf("字幕条目%s的 before 与当前字幕不一致", ch.CueID)
		}
		ch.After.ID = current.ID
		ch.After.PackageID = current.PackageID
		ch.After.Sequence = current.Sequence
		ch.After.Revision = current.Revision + 1
		diff := domain.CompareCue(current, ch.After)
		if len(diff.Fields) == 0 {
			return nil, impact, fmt.Errorf("修订条目%s没有实际变化", ch.CueID)
		}
		if _, ok := diff.Fields["startMs"]; ok {
			timeChanged[ch.CueID] = true
		}
		if _, ok := diff.Fields["endMs"]; ok {
			timeChanged[ch.CueID] = true
		}
		projected.Cues[idx] = ch.After
		impact.Diffs = append(impact.Diffs, diff)
	}
	if err := ValidateCueSet(&projected); err != nil {
		return nil, impact, fmt.Errorf("修订后字幕无效: %w", err)
	}

	selected := map[string]bool{}
	for _, id := range findingIDs {
		if selected[id] {
			return nil, impact, fmt.Errorf("findingId %s 重复", id)
		}
		selected[id] = true
	}
	linkable := map[string]bool{}
	for _, finding := range p.Findings {
		if finding.Disposition != "open" && finding.Disposition != "question" {
			continue
		}
		for _, ch := range normalized {
			if findingRelatesToCue(finding, ch.Before) {
				linkable[finding.ID] = true
				impact.LinkableFindings = append(impact.LinkableFindings, finding)
				break
			}
		}
	}
	for id := range selected {
		if !linkable[id] {
			return nil, impact, fmt.Errorf("findingId %s 不存在、已解决或与改动字幕无关", id)
		}
	}
	for _, finding := range p.Findings {
		if finding.Disposition != "open" && finding.Disposition != "question" {
			continue
		}
		covered := linkable[finding.ID]
		if len(selected) > 0 {
			covered = selected[finding.ID]
		}
		if !covered {
			impact.Uncovered = append(impact.Uncovered, finding)
		}
	}
	adjacent := map[string]bool{}
	for _, cue := range p.Cues {
		if !timeChanged[cue.ID] {
			continue
		}
		for _, neighbor := range p.Cues {
			if neighbor.Sequence == cue.Sequence-1 || neighbor.Sequence == cue.Sequence+1 {
				adjacent[neighbor.ID] = true
			}
		}
	}
	for _, cue := range p.Cues {
		if adjacent[cue.ID] {
			impact.AdjacentCues = append(impact.AdjacentCues, cue)
		}
	}
	return normalized, impact, nil
}

func findingRelatesToCue(finding domain.ReviewFinding, cue domain.CaptionCue) bool {
	if finding.CueID != "" {
		return finding.CueID == cue.ID
	}
	return finding.StartMs < cue.EndMs && finding.EndMs > cue.StartMs
}

func (s *Service) PreviewFreeze(id string) (domain.FreezePreview, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return domain.FreezePreview{}, fmt.Errorf("字幕包不存在")
	}
	if p.Status != domain.StatusReviewPassed {
		return domain.FreezePreview{}, fmt.Errorf("只有 review_passed 状态可以预检冻结")
	}
	if unresolved := domain.Unresolved(p); len(unresolved) > 0 {
		return domain.FreezePreview{}, fmt.Errorf("存在%d项未解决发现，不能预检冻结", len(unresolved))
	}
	if err := ValidateCueSet(p); err != nil {
		return domain.FreezePreview{}, fmt.Errorf("字幕数据无效: %w", err)
	}
	content, summary, checksum := domain.WebVTT(p)
	return domain.FreezePreview{Content: content, Summary: summary, CueCount: len(p.Cues), ExpectedVersion: p.Version, Checksum: checksum}, nil
}

func (s *Service) QualityStatistics(filter StatisticsFilter) domain.QualityStatistics {
	result := domain.QualityStatistics{Findings: []domain.FindingDistribution{}}
	matched := map[string]bool{}
	packages := s.Store.List()
	for _, p := range packages {
		if !filter.From.IsZero() && p.UpdatedAt.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && p.UpdatedAt.After(filter.To) {
			continue
		}
		if filter.Language != "" && domain.NormalizeLanguage(p.Language) != domain.NormalizeLanguage(filter.Language) {
			continue
		}
		if filter.DeliveryStandard != "" && p.DeliveryStandard != filter.DeliveryStandard {
			continue
		}
		matched[p.ID] = true
		result.PackageCount++
		if p.Status == domain.StatusDelivered {
			result.DeliveredCount++
		}
		result.RevisionCount += len(p.Revisions)
	}

	type distributionKey struct{ rule, severity, source, disposition string }
	distributions := map[distributionKey]int{}
	for _, p := range packages {
		if !matched[p.ID] {
			continue
		}
		for _, finding := range p.Findings {
			key := distributionKey{finding.RuleCode, finding.Severity, finding.Source, finding.Disposition}
			distributions[key]++
		}
	}
	for key, count := range distributions {
		result.Findings = append(result.Findings, domain.FindingDistribution{RuleCode: key.rule, Severity: key.severity, Source: key.source, Disposition: key.disposition, Count: count})
	}
	sort.Slice(result.Findings, func(i, j int) bool {
		a, b := result.Findings[i], result.Findings[j]
		if a.RuleCode != b.RuleCode {
			return a.RuleCode < b.RuleCode
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.Disposition < b.Disposition
	})
	seenReview := map[string]bool{}
	for _, event := range s.Store.Events("") {
		if !matched[event.PackageID] {
			continue
		}
		switch event.Type {
		case domain.EventReviewSubmitted:
			resultValue := fmt.Sprint(event.Payload["result"])
			if !seenReview[event.PackageID] && resultValue == string(domain.StatusReviewPassed) {
				result.FirstPassCount++
			}
			if resultValue == string(domain.StatusCorrectionRequired) {
				result.ReturnedCount++
			}
			seenReview[event.PackageID] = true
		case domain.EventRevisionCreated:
			result.ReworkBatchCount++
		}
	}
	return result
}
