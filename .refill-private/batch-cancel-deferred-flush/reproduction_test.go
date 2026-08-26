package batch_cancel_deferred_flush_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/httpapi"
	"subtitleqc/internal/store"
	"subtitleqc/internal/workflow"
)

type cancelDuringBatchContext struct {
	context.Context
	done   chan struct{}
	once   sync.Once
	checks int
}

func newCancelDuringBatchContext() *cancelDuringBatchContext {
	return &cancelDuringBatchContext{Context: context.Background(), done: make(chan struct{})}
}

func (c *cancelDuringBatchContext) Done() <-chan struct{} { return c.done }

func (c *cancelDuringBatchContext) Err() error {
	c.checks++
	if c.checks < 2 {
		return nil
	}
	c.once.Do(func() { close(c.done) })
	return context.Canceled
}

func TestCanceledFindingBatchLeavesNoPartialCommit(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := workflow.New(st)
	p, err := app.Create(workflow.CreateRequest{
		ProgramTitle:     "取消批处理测试",
		EpisodeCode:      "CANCEL-BATCH",
		AudioDurationMs:  10000,
		Language:         "zh-CN",
		DeliveryStandard: "WebVTT",
	}, "editor")
	if err != nil {
		t.Fatal(err)
	}
	p, err = app.Prepare(p.ID, workflow.CueRequest{
		ExpectedVersion: p.Version,
		IdempotencyKey:  "prepare-cancel-batch",
		Cues: []domain.CaptionCue{
			{ID: "cue-cancel-batch-1", Sequence: 1, StartMs: 0, EndMs: 100, Text: "这是一段会触发阅读速度检查的字幕", SoundHint: "[音乐]"},
			{ID: "cue-cancel-batch-2", Sequence: 2, StartMs: 200, EndMs: 300, Text: "第二段同样会触发阅读速度检查", SoundHint: "[音乐]"},
		},
	}, "editor")
	if err != nil {
		t.Fatal(err)
	}
	p, err = app.Check(p.ID, p.Version, "check-cancel-batch", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Findings) < 3 {
		t.Fatalf("测试前置发现不足: %+v", p.Findings)
	}

	beforeVersion := p.Version
	beforeEvents := len(st.Events(p.ID))
	body, err := json.Marshal(map[string]any{
		"expectedVersion": beforeVersion,
		"idempotencyKey":  "cancelled-finding-batch",
		"role":            "reviewer",
		"findings": []map[string]any{
			{"findingId": p.Findings[0].ID, "disposition": "resolved"},
			{"findingId": p.Findings[1].ID, "disposition": "resolved"},
			{"findingId": p.Findings[2].ID, "disposition": "resolved"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := newCancelDuringBatchContext()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subtitle-packages/"+p.ID+"/findings", strings.NewReader(string(body))).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	httpapi.New(app).Handler().ServeHTTP(recorder, req)
	if recorder.Code == http.StatusOK {
		t.Fatalf("已取消批次不应返回成功: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), context.Canceled.Error()) {
		t.Fatalf("响应没有报告 context 取消: %s", recorder.Body.String())
	}

	current, ok := st.Get(p.ID)
	if !ok {
		t.Fatal("字幕包在取消后丢失")
	}
	changed := 0
	for i := 0; i < 3; i++ {
		if current.Findings[i].Disposition != "open" {
			changed++
		}
	}
	if current.Version != beforeVersion || len(st.Events(p.ID)) != beforeEvents || changed != 0 {
		t.Fatalf("已取消批次发生部分提交: version=%d addedEvents=%d changedFindings=%d", current.Version, len(st.Events(p.ID))-beforeEvents, changed)
	}
}
