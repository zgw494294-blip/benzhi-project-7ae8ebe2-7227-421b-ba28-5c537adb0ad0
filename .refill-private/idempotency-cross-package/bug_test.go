package idempotencycrosspackage

import (
	"testing"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/store"
	"subtitleqc/internal/workflow"
)

func TestSharedIdempotencyKeyCannotCrossPackages(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := workflow.New(st)

	first, err := app.Create(workflow.CreateRequest{ProgramTitle: "节目一", EpisodeCode: "EP1", AudioDurationMs: 5000, Language: "zh-CN", DeliveryStandard: "WebVTT"}, "editor")
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.Create(workflow.CreateRequest{ProgramTitle: "节目二", EpisodeCode: "EP2", AudioDurationMs: 5000, Language: "zh-CN", DeliveryStandard: "WebVTT"}, "editor")
	if err != nil {
		t.Fatal(err)
	}
	cue := func(id string) []domain.CaptionCue {
		return []domain.CaptionCue{{ID: id, Sequence: 1, StartMs: 0, EndMs: 1000, Speaker: "主持人", Text: "字幕"}}
	}
	if _, err = app.Prepare(first.ID, workflow.CueRequest{ExpectedVersion: first.Version, IdempotencyKey: "shared-key", Cues: cue("first-cue")}, "editor"); err != nil {
		t.Fatal(err)
	}
	if _, err = app.Prepare(second.ID, workflow.CueRequest{ExpectedVersion: second.Version, IdempotencyKey: "shared-key", Cues: cue("second-cue")}, "editor"); err == nil {
		t.Fatalf("跨字幕包复用同一幂等键被静默接受")
	}
}
