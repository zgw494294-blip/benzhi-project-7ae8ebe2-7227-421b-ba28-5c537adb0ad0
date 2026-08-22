package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/store"
)

type Service struct{ Store *store.Store }

func New(s *store.Store) *Service { return &Service{Store: s} }

type CreateRequest struct {
	ProgramTitle     string `json:"programTitle"`
	EpisodeCode      string `json:"episodeCode"`
	AudioDurationMs  int64  `json:"audioDurationMs"`
	Language         string `json:"language"`
	DeliveryStandard string `json:"deliveryStandard"`
}
type CueRequest struct {
	ExpectedVersion int                 `json:"expectedVersion"`
	IdempotencyKey  string              `json:"idempotencyKey"`
	Cues            []domain.CaptionCue `json:"cues"`
}
type FindingDisposition struct {
	FindingID      string `json:"findingId"`
	Disposition    string `json:"disposition"`
	ResolutionNote string `json:"resolutionNote"`
}

func (s *Service) Create(req CreateRequest, actor string) (*domain.SubtitlePackage, error) {
	p := &domain.SubtitlePackage{ID: fmt.Sprintf("pkg-%d", time.Now().UnixNano()), ProgramTitle: req.ProgramTitle, EpisodeCode: req.EpisodeCode, AudioDurationMs: req.AudioDurationMs, Language: req.Language, DeliveryStandard: req.DeliveryStandard, Status: domain.StatusDraft, Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Cues: []domain.CaptionCue{}, Findings: []domain.ReviewFinding{}, Revisions: []domain.RevisionBatch{}}
	if err := p.ValidateMetadata(); err != nil {
		return nil, err
	}
	if err := s.Store.Commit(p.ID, "", 0, p, domain.EventPackageCreated, actor, map[string]any{"status": p.Status}); err != nil {
		return nil, err
	}
	stored, _ := s.Store.Get(p.ID)
	return stored, nil
}

func (s *Service) PreviewImport(id, raw string) ([]domain.CaptionCue, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("字幕包不存在")
	}
	if p.Status != domain.StatusDraft {
		return nil, fmt.Errorf("当前状态不能预览导入")
	}
	cues, err := domain.ParseCueLines(id, raw)
	if err != nil {
		return nil, err
	}
	p.Cues = cues
	if err := ValidateCueSet(p); err != nil {
		return nil, err
	}
	for i := range p.Cues {
		p.Cues[i].PackageID = id
		if p.Cues[i].Revision == 0 {
			p.Cues[i].Revision = 1
		}
	}
	return p.Cues, nil
}
func (s *Service) Prepare(id string, req CueRequest, actor string) (*domain.SubtitlePackage, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("字幕包不存在")
	}
	if s.Store.HasIdempotency(req.IdempotencyKey) {
		return p, nil
	}
	if p.Status != domain.StatusDraft {
		return nil, fmt.Errorf("当前状态不能录入条目")
	}
	if len(req.Cues) == 0 {
		return nil, fmt.Errorf("至少需要一条字幕")
	}
	for i := range req.Cues {
		req.Cues[i].ID = nonEmpty(req.Cues[i].ID, fmt.Sprintf("cue-%d-%d", time.Now().UnixNano(), i))
		req.Cues[i].PackageID = id
		if req.Cues[i].EndMs <= req.Cues[i].StartMs {
			return nil, fmt.Errorf("条目%d时间码无效", i+1)
		}
	}
	p.Cues = req.Cues
	if err := ValidateCueSet(p); err != nil {
		return nil, err
	}
	if err := domain.Transition(p, domain.StatusPrepared); err != nil {
		return nil, err
	}
	if err := s.Store.Commit(id, req.IdempotencyKey, req.ExpectedVersion, p, domain.EventCuesPrepared, actor, map[string]any{"cueCount": len(p.Cues)}); err != nil {
		return nil, err
	}
	stored, _ := s.Store.Get(id)
	return stored, nil
}
func (s *Service) Check(id string, expected int, key, actor string) (*domain.SubtitlePackage, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("字幕包不存在")
	}
	if s.Store.HasIdempotency(key) {
		return p, nil
	}
	if p.Status != domain.StatusPrepared && !(p.Status == domain.StatusReviewing && len(p.Revisions) > 0) {
		return nil, fmt.Errorf("当前状态不能质检")
	}
	automatic := domain.RunQualityChecks(p)
	if p.Status == domain.StatusPrepared {
		p.Findings = automatic
	} else {
		known := make(map[string]bool, len(p.Findings))
		for _, finding := range p.Findings {
			known[finding.ID] = true
		}
		for _, finding := range automatic {
			if !known[finding.ID] {
				p.Findings = append(p.Findings, finding)
			}
		}
	}
	if p.Status == domain.StatusPrepared {
		if err := domain.Transition(p, domain.StatusReviewing); err != nil {
			return nil, err
		}
	} else {
		p.Version++
		p.UpdatedAt = time.Now().UTC()
	}
	if err := s.Store.Commit(id, key, expected, p, domain.EventQualityChecked, actor, map[string]any{"findings": len(p.Findings)}); err != nil {
		return nil, err
	}
	stored, _ := s.Store.Get(id)
	return stored, nil
}
func (s *Service) Disposition(id, findingID, disposition, note, actor string) (*domain.SubtitlePackage, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("字幕包不存在")
	}
	if p.Status != domain.StatusReviewing {
		return nil, fmt.Errorf("当前状态不能处理发现")
	}
	var found *domain.ReviewFinding
	for i := range p.Findings {
		if p.Findings[i].ID == findingID {
			found = &p.Findings[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("发现不存在")
	}
	if found.Disposition != "open" {
		return nil, fmt.Errorf("发现已处置")
	}
	if disposition != "resolved" && disposition != "waived" && disposition != "question" {
		return nil, fmt.Errorf("无效处置")
	}
	if (disposition == "waived" || disposition == "question") && strings.TrimSpace(note) == "" {
		return nil, fmt.Errorf("该处置必须填写说明")
	}
	found.Disposition = disposition
	found.ResolutionNote = note
	found.ResolvedBy = actor
	t := time.Now().UTC()
	found.ResolvedAt = &t
	p.Version++
	p.UpdatedAt = t
	return s.commitDirect(id, p, domain.EventFindingDispositioned, actor, map[string]any{"findingId": findingID, "disposition": disposition, "resolutionNote": note})
}

func (s *Service) BatchDisposition(id string, expected int, key, actor, role string, items []FindingDisposition) (*domain.SubtitlePackage, error) {
	return s.BatchDispositionContext(context.Background(), id, expected, key, actor, role, items)
}

func (s *Service) BatchDispositionContext(ctx context.Context, id string, expected int, key, actor, role string, items []FindingDisposition) (result *domain.SubtitlePackage, err error) {
	if role != "" && !domainRoleAllows(role, "finding") {
		return nil, fmt.Errorf("角色%s无权执行finding", role)
	}
	p, ok := s.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("字幕包不存在")
	}
	if s.Store.HasIdempotency(key) {
		return p, nil
	}
	if p.Status != domain.StatusReviewing {
		return nil, fmt.Errorf("当前状态不能处理发现")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("至少需要一条处置")
	}
	seen := map[string]bool{}
	for _, item := range items {
		if seen[item.FindingID] {
			return nil, fmt.Errorf("发现重复")
		}
		seen[item.FindingID] = true
		var found *domain.ReviewFinding
		for i := range p.Findings {
			if p.Findings[i].ID == item.FindingID {
				found = &p.Findings[i]
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("发现%s不存在", item.FindingID)
		}
		if found.Disposition != "open" {
			return nil, fmt.Errorf("发现%s已处置", item.FindingID)
		}
		if item.Disposition != "resolved" && item.Disposition != "waived" && item.Disposition != "question" {
			return nil, fmt.Errorf("无效处置")
		}
		if (item.Disposition == "waived" || item.Disposition == "question") && strings.TrimSpace(item.ResolutionNote) == "" {
			return nil, fmt.Errorf("该处置必须填写说明")
		}
	}
	t := time.Now().UTC()
	for _, item := range items {
		for i := range p.Findings {
			if p.Findings[i].ID == item.FindingID {
				p.Findings[i].Disposition = item.Disposition
				p.Findings[i].ResolutionNote = strings.TrimSpace(item.ResolutionNote)
				p.Findings[i].ResolvedBy = actor
				p.Findings[i].ResolvedAt = &t
			}
		}
	}
	p.Version++
	p.UpdatedAt = t
	records := make([]store.EventRecord, 0, len(items))
	canceled := false
	defer func() {
		if canceled && len(records) > 0 {
			_ = s.Store.CommitMany(id, key, expected, p, records)
		}
	}()
	for _, item := range items {
		record := store.EventRecord{Type: domain.EventFindingDispositioned, Actor: actor, Payload: map[string]any{"findingId": item.FindingID, "disposition": item.Disposition, "resolutionNote": strings.TrimSpace(item.ResolutionNote)}}
		records = append(records, record)
		if cancelErr := ctx.Err(); cancelErr != nil {
			canceled = true
			return nil, cancelErr
		}
	}
	if err := s.Store.CommitMany(id, key, expected, p, records); err != nil {
		return nil, err
	}
	stored, _ := s.Store.Get(id)
	return stored, nil
}
func (s *Service) SubmitReview(id string, expected int, key, actor string) (*domain.SubtitlePackage, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("字幕包不存在")
	}
	if s.Store.HasIdempotency(key) {
		return p, nil
	}
	if p.Status != domain.StatusReviewing {
		return nil, fmt.Errorf("当前状态不能提交复审")
	}
	if revisionNeedsQualityCheck(s.Store.Events(id)) {
		return nil, fmt.Errorf("修订后必须先重新质检")
	}
	target := domain.StatusReviewPassed
	if len(domain.Unresolved(p)) > 0 {
		target = domain.StatusCorrectionRequired
	}
	if err := domain.Transition(p, target); err != nil {
		return nil, err
	}
	if err := s.Store.Commit(id, key, expected, p, domain.EventReviewSubmitted, actor, map[string]any{"result": target}); err != nil {
		return nil, err
	}
	stored, _ := s.Store.Get(id)
	return stored, nil
}

func revisionNeedsQualityCheck(events []domain.AuditEvent) bool {
	var revisionSequence, checkSequence int64
	for _, event := range events {
		switch event.Type {
		case domain.EventRevisionCreated:
			revisionSequence = event.Sequence
		case domain.EventQualityChecked:
			checkSequence = event.Sequence
		}
	}
	return revisionSequence > checkSequence
}
func (s *Service) Revise(id string, expected int, key, reason, actor string, changes []domain.RevisionChange, findingIDs []string) (*domain.SubtitlePackage, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("字幕包不存在")
	}
	if s.Store.HasIdempotency(key) {
		return p, nil
	}
	if err := ensureExpected(p.Version, expected); err != nil {
		return nil, err
	}
	if p.Status != domain.StatusCorrectionRequired {
		return nil, fmt.Errorf("当前状态不能修订")
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("idempotencyKey 不能为空")
	}
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("修订原因不能为空")
	}
	if len(changes) == 0 {
		return nil, fmt.Errorf("修订内容不能为空")
	}
	changes, impact, err := validateRevision(p, changes, findingIDs)
	if err != nil {
		return nil, err
	}
	for _, ch := range changes {
		for i := range p.Cues {
			if p.Cues[i].ID == ch.CueID {
				p.Cues[i] = ch.After
			}
		}
	}
	now := time.Now().UTC()
	selected := make(map[string]bool, len(findingIDs))
	for _, findingID := range findingIDs {
		selected[findingID] = true
	}
	for i := range p.Findings {
		if selected[p.Findings[i].ID] {
			p.Findings[i].Disposition = "resolved"
			p.Findings[i].ResolutionNote = "由修订批次提供解决证据"
			p.Findings[i].ResolvedBy = actor
			p.Findings[i].ResolvedAt = &now
		}
	}
	p.Revisions = append(p.Revisions, domain.RevisionBatch{ID: fmt.Sprintf("rev-%d", time.Now().UnixNano()), PackageID: id, BaseVersion: p.Version, Reason: strings.TrimSpace(reason), Changes: changes, FindingIDs: append([]string(nil), findingIDs...), SubmittedBy: actor, SubmittedAt: now})
	if err := domain.Transition(p, domain.StatusReviewing); err != nil {
		return nil, err
	}
	if err := s.Store.Commit(id, key, expected, p, domain.EventRevisionCreated, actor, map[string]any{"changes": len(changes), "diffs": impact.Diffs, "resolvedFindingIds": findingIDs, "baseVersion": expected}); err != nil {
		return nil, err
	}
	stored, _ := s.Store.Get(id)
	return stored, nil
}
func (s *Service) Freeze(id string, expected int, key, actor string) (*domain.SubtitlePackage, error) {
	preview, err := s.PreviewFreeze(id)
	if err != nil {
		return nil, err
	}
	return s.FreezeConfirmed(id, expected, preview.Checksum, key, actor)
}

func (s *Service) FreezeConfirmed(id string, expected int, expectedChecksum, key, actor string) (*domain.SubtitlePackage, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("字幕包不存在")
	}
	if s.Store.HasIdempotency(key) {
		return p, nil
	}
	if err := ensureExpected(p.Version, expected); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("idempotencyKey 不能为空")
	}
	if p.Status != domain.StatusReviewPassed {
		return nil, fmt.Errorf("只有复审通过才能冻结")
	}
	if len(domain.Unresolved(p)) > 0 {
		return nil, fmt.Errorf("仍有未解决发现，不能冻结")
	}
	if err := ValidateCueSet(p); err != nil {
		return nil, err
	}
	content, summary, sum := domain.WebVTT(p)
	if strings.TrimSpace(expectedChecksum) == "" {
		return nil, fmt.Errorf("expectedChecksum 不能为空，请先执行冻结预检")
	}
	if sum != expectedChecksum {
		return nil, fmt.Errorf("冻结内容校验和已变化: 期望%s，当前%s", expectedChecksum, sum)
	}
	p.Master = &domain.FrozenMaster{Format: "WebVTT", Content: content, Summary: summary, Checksum: sum, FrozenAt: time.Now().UTC()}
	if err := domain.Transition(p, domain.StatusFrozen); err != nil {
		return nil, err
	}
	if err := s.Store.Commit(id, key, expected, p, domain.EventMasterFrozen, actor, map[string]any{"checksum": sum}); err != nil {
		return nil, err
	}
	stored, _ := s.Store.Get(id)
	return stored, nil
}
func (s *Service) Deliver(id string, expected int, key, actor string) (*domain.SubtitlePackage, error) {
	p, ok := s.Store.Get(id)
	if !ok {
		return nil, fmt.Errorf("字幕包不存在")
	}
	if s.Store.HasIdempotency(key) {
		return p, nil
	}
	if p.Status != domain.StatusFrozen || p.Master == nil {
		return nil, fmt.Errorf("只有冻结母版才能交付")
	}
	if _, _, checksum := domain.WebVTT(p); checksum != p.Master.Checksum || domain.ContentChecksum(p.Master.Content) != p.Master.Checksum {
		return nil, fmt.Errorf("冻结母版完整性校验失败")
	}
	if p.Credential != nil {
		return nil, fmt.Errorf("交付凭据已签发")
	}
	p.Credential = &domain.DeliveryCredential{ID: fmt.Sprintf("cred-%d", time.Now().UnixNano()), PackageID: id, FrozenVersion: p.Version, MasterFormat: p.Master.Format, MasterChecksum: p.Master.Checksum, CueCount: len(p.Cues), ApprovedBy: actor, IssuedAt: time.Now().UTC()}
	if err := domain.Transition(p, domain.StatusDelivered); err != nil {
		return nil, err
	}
	if err := s.Store.Commit(id, key, expected, p, domain.EventCredentialIssued, actor, map[string]any{"credentialId": p.Credential.ID}); err != nil {
		return nil, err
	}
	stored, _ := s.Store.Get(id)
	return stored, nil
}
func (s *Service) commitDirect(id string, p *domain.SubtitlePackage, typ, actor string, payload map[string]any) (*domain.SubtitlePackage, error) {
	return s.commitDirectWithKey(id, p, "", p.Version-1, typ, actor, payload)
}
func (s *Service) commitDirectWithKey(id string, p *domain.SubtitlePackage, key string, expected int, typ, actor string, payload map[string]any) (*domain.SubtitlePackage, error) {
	if err := s.Store.Commit(id, key, expected, p, typ, actor, payload); err != nil {
		return nil, err
	}
	stored, _ := s.Store.Get(id)
	return stored, nil
}
func nonEmpty(v, f string) string {
	if strings.TrimSpace(v) == "" {
		return f
	}
	return v
}
