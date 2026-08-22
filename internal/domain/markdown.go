package domain

import "strings"

func PlainText(p *SubtitlePackage) string {
	var b strings.Builder
	b.WriteString(p.ProgramTitle)
	b.WriteString(" ")
	b.WriteString(p.EpisodeCode)
	b.WriteByte('\n')
	for _, c := range p.Cues {
		b.WriteString(c.Text)
		b.WriteByte('\n')
	}
	return b.String()
}
func CueCountBySpeaker(p *SubtitlePackage) map[string]int {
	m := map[string]int{}
	for _, c := range p.Cues {
		m[c.Speaker]++
	}
	return m
}
