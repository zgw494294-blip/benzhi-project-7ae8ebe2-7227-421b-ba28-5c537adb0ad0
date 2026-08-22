package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Status string

const (
	StatusDraft              Status = "draft"
	StatusPrepared           Status = "prepared"
	StatusReviewing          Status = "reviewing"
	StatusCorrectionRequired Status = "correction_required"
	StatusReviewPassed       Status = "review_passed"
	StatusFrozen             Status = "frozen"
	StatusDelivered          Status = "delivered"
)

type SubtitlePackage struct {
	ID               string              `json:"id"`
	ProgramTitle     string              `json:"programTitle"`
	EpisodeCode      string              `json:"episodeCode"`
	AudioDurationMs  int64               `json:"audioDurationMs"`
	Language         string              `json:"language"`
	DeliveryStandard string              `json:"deliveryStandard"`
	Status           Status              `json:"status"`
	Version          int                 `json:"version"`
	CreatedAt        time.Time           `json:"createdAt"`
	UpdatedAt        time.Time           `json:"updatedAt"`
	Cues             []CaptionCue        `json:"cues"`
	Findings         []ReviewFinding     `json:"findings"`
	Revisions        []RevisionBatch     `json:"revisions"`
	Credential       *DeliveryCredential `json:"credential,omitempty"`
	Master           *FrozenMaster       `json:"master,omitempty"`
}

type CaptionCue struct {
	ID        string `json:"id"`
	PackageID string `json:"packageId"`
	Sequence  int    `json:"sequence"`
	StartMs   int64  `json:"startMs"`
	EndMs     int64  `json:"endMs"`
	Speaker   string `json:"speaker"`
	Text      string `json:"text"`
	SoundHint string `json:"soundHint"`
	Revision  int    `json:"revision"`
}

type ReviewFinding struct {
	ID             string     `json:"id"`
	PackageID      string     `json:"packageId"`
	CueID          string     `json:"cueId,omitempty"`
	StartMs        int64      `json:"startMs,omitempty"`
	EndMs          int64      `json:"endMs,omitempty"`
	Source         string     `json:"source"`
	RuleCode       string     `json:"ruleCode"`
	Severity       string     `json:"severity"`
	Message        string     `json:"message"`
	Disposition    string     `json:"disposition"`
	ReviewRound    int        `json:"reviewRound"`
	CreatedBy      string     `json:"createdBy,omitempty"`
	CreatedAt      *time.Time `json:"createdAt,omitempty"`
	ResolutionNote string     `json:"resolutionNote,omitempty"`
	ResolvedBy     string     `json:"resolvedBy,omitempty"`
	ResolvedAt     *time.Time `json:"resolvedAt,omitempty"`
}

type RevisionChange struct {
	CueID  string     `json:"cueId"`
	Before CaptionCue `json:"before"`
	After  CaptionCue `json:"after"`
	Reason string     `json:"reason"`
}

type RevisionBatch struct {
	ID          string           `json:"id"`
	PackageID   string           `json:"packageId"`
	BaseVersion int              `json:"baseVersion"`
	Reason      string           `json:"reason"`
	Changes     []RevisionChange `json:"changes"`
	FindingIDs  []string         `json:"findingIds"`
	SubmittedBy string           `json:"submittedBy"`
	SubmittedAt time.Time        `json:"submittedAt"`
}

type DeliveryCredential struct {
	ID             string    `json:"id"`
	PackageID      string    `json:"packageId"`
	FrozenVersion  int       `json:"frozenVersion"`
	MasterFormat   string    `json:"masterFormat"`
	MasterChecksum string    `json:"masterChecksum"`
	CueCount       int       `json:"cueCount"`
	ApprovedBy     string    `json:"approvedBy"`
	IssuedAt       time.Time `json:"issuedAt"`
}

type FrozenMaster struct {
	Format   string    `json:"format"`
	Content  string    `json:"content"`
	Summary  string    `json:"summary"`
	Checksum string    `json:"checksum"`
	FrozenAt time.Time `json:"frozenAt"`
}

type AuditEvent struct {
	Sequence  int64          `json:"sequence"`
	ID        string         `json:"id"`
	PackageID string         `json:"packageId"`
	Type      string         `json:"type"`
	Actor     string         `json:"actor"`
	At        time.Time      `json:"at"`
	Payload   map[string]any `json:"payload"`
	Checksum  string         `json:"checksum"`
}

func (p *SubtitlePackage) NormalizeCues() {
	sort.SliceStable(p.Cues, func(i, j int) bool { return p.Cues[i].Sequence < p.Cues[j].Sequence })
	for i := range p.Cues {
		p.Cues[i].Sequence = i + 1
	}
}

func (p *SubtitlePackage) ValidateMetadata() error {
	if strings.TrimSpace(p.ProgramTitle) == "" {
		return fmt.Errorf("节目标题不能为空")
	}
	if strings.TrimSpace(p.EpisodeCode) == "" {
		return fmt.Errorf("期号不能为空")
	}
	if err := ValidateEpisodeCode(p.EpisodeCode); err != nil {
		return err
	}
	if p.AudioDurationMs <= 0 {
		return fmt.Errorf("音频时长必须为正数")
	}
	if strings.TrimSpace(p.Language) == "" {
		return fmt.Errorf("语言不能为空")
	}
	if strings.TrimSpace(p.DeliveryStandard) == "" {
		return fmt.Errorf("无障碍交付标准不能为空")
	}
	if err := ValidateStandard(p.DeliveryStandard); err != nil {
		return err
	}
	return nil
}

func AllowedTransition(from, to Status) bool {
	switch from {
	case StatusDraft:
		return to == StatusPrepared
	case StatusPrepared:
		return to == StatusReviewing
	case StatusReviewing:
		return to == StatusCorrectionRequired || to == StatusReviewPassed
	case StatusCorrectionRequired:
		return to == StatusReviewing
	case StatusReviewPassed:
		return to == StatusFrozen
	case StatusFrozen:
		return to == StatusDelivered
	}
	return false
}

func Transition(p *SubtitlePackage, to Status) error {
	if !AllowedTransition(p.Status, to) {
		return fmt.Errorf("不允许从 %s 迁移到 %s", p.Status, to)
	}
	p.Status = to
	p.Version++
	p.UpdatedAt = time.Now().UTC()
	return nil
}

func WebVTT(p *SubtitlePackage) (string, string, string) {
	p.NormalizeCues()
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, c := range p.Cues {
		b.WriteString(fmt.Sprintf("%02d\n%s --> %s\n", c.Sequence, formatTime(c.StartMs), formatTime(c.EndMs)))
		if c.Speaker != "" {
			b.WriteString("<v " + c.Speaker + ">")
		}
		b.WriteString(c.Text)
		if c.SoundHint != "" {
			b.WriteString("\n")
			if strings.HasPrefix(c.SoundHint, "[") && strings.HasSuffix(c.SoundHint, "]") {
				b.WriteString(c.SoundHint)
			} else {
				b.WriteString("[" + c.SoundHint + "]")
			}
		}
		b.WriteString("\n\n")
	}
	content := b.String()
	sum := sha256.Sum256([]byte(content))
	checksum := hex.EncodeToString(sum[:])
	summary := fmt.Sprintf("%s %s，共%d条字幕，时长%s", p.ProgramTitle, p.EpisodeCode, len(p.Cues), formatTime(p.AudioDurationMs))
	return content, summary, checksum
}

func ContentChecksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func formatTime(ms int64) string {
	h := ms / 3600000
	m := (ms % 3600000) / 60000
	s := (ms % 60000) / 1000
	milli := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, milli)
}
