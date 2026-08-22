package domain

type RevisionImpact struct {
	Diffs            []CueDiff       `json:"diffs"`
	LinkableFindings []ReviewFinding `json:"linkableFindings"`
	Uncovered        []ReviewFinding `json:"uncoveredFindings"`
	AdjacentCues     []CaptionCue    `json:"adjacentCues"`
}

type FreezePreview struct {
	Content         string `json:"content"`
	Summary         string `json:"summary"`
	CueCount        int    `json:"cueCount"`
	ExpectedVersion int    `json:"expectedVersion"`
	Checksum        string `json:"checksum"`
}

type FindingDistribution struct {
	RuleCode    string `json:"ruleCode"`
	Severity    string `json:"severity"`
	Source      string `json:"source"`
	Disposition string `json:"disposition"`
	Count       int    `json:"count"`
}

type QualityStatistics struct {
	PackageCount     int                   `json:"packageCount"`
	DeliveredCount   int                   `json:"deliveredCount"`
	RevisionCount    int                   `json:"revisionCount"`
	FirstPassCount   int                   `json:"firstPassCount"`
	ReturnedCount    int                   `json:"returnedCount"`
	ReworkBatchCount int                   `json:"reworkBatchCount"`
	Findings         []FindingDistribution `json:"findings"`
}
