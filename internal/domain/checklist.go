package domain

import "sort"

type Checklist struct {
	Total      int            `json:"total"`
	Errors     int            `json:"errors"`
	Warnings   int            `json:"warnings"`
	Resolved   int            `json:"resolved"`
	Open       int            `json:"open"`
	Question   int            `json:"question"`
	Waived     int            `json:"waived"`
	Blocked    bool           `json:"blocking"`
	ByRule     map[string]int `json:"byRule"`
	BySeverity map[string]int `json:"bySeverity"`
}

func BuildChecklist(p *SubtitlePackage) Checklist {
	c := Checklist{ByRule: map[string]int{}, BySeverity: map[string]int{}}
	for _, f := range p.Findings {
		c.Total++
		c.ByRule[f.RuleCode]++
		c.BySeverity[f.Severity]++
		if f.Severity == "error" {
			c.Errors++
		}
		if f.Severity == "warning" {
			c.Warnings++
		}
		switch f.Disposition {
		case "open":
			c.Open++
		case "question":
			c.Question++
		case "waived":
			c.Waived++
		}
		if f.Disposition == "resolved" || f.Disposition == "waived" {
			c.Resolved++
		}
	}
	c.Blocked = c.Open+c.Question > 0 && c.BySeverity["error"] > 0
	return c
}
func (c Checklist) Complete() bool { return c.Total == c.Resolved }
func (c Checklist) Blocking() bool { return c.Blocked }

type FindingGroup struct {
	RuleCode string `json:"ruleCode"`
	Severity string `json:"severity"`
	Count    int    `json:"count"`
}

func (c Checklist) Groups() []FindingGroup {
	keys := make([]string, 0, len(c.ByRule))
	for k := range c.ByRule {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]FindingGroup, 0, len(keys))
	for _, k := range keys {
		out = append(out, FindingGroup{RuleCode: k, Count: c.ByRule[k]})
	}
	return out
}
