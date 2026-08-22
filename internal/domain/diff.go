package domain

import "fmt"

type CueDiff struct {
	CueID  string               `json:"cueId"`
	Fields map[string][2]string `json:"fields"`
}

func CompareCue(before, after CaptionCue) CueDiff {
	d := CueDiff{CueID: after.ID, Fields: map[string][2]string{}}
	if before.StartMs != after.StartMs {
		d.Fields["startMs"] = pair(fmt.Sprint(before.StartMs), fmt.Sprint(after.StartMs))
	}
	if before.EndMs != after.EndMs {
		d.Fields["endMs"] = pair(fmt.Sprint(before.EndMs), fmt.Sprint(after.EndMs))
	}
	if before.Speaker != after.Speaker {
		d.Fields["speaker"] = pair(before.Speaker, after.Speaker)
	}
	if before.Text != after.Text {
		d.Fields["text"] = pair(before.Text, after.Text)
	}
	if before.SoundHint != after.SoundHint {
		d.Fields["soundHint"] = pair(before.SoundHint, after.SoundHint)
	}
	return d
}
func pair(a, b string) [2]string { return [2]string{a, b} }
func CompareBatch(b RevisionBatch) []CueDiff {
	out := make([]CueDiff, 0, len(b.Changes))
	for _, c := range b.Changes {
		out = append(out, CompareCue(c.Before, c.After))
	}
	return out
}
