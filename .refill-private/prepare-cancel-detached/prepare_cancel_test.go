package preparecanceltest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/httpapi"
	"subtitleqc/internal/store"
	"subtitleqc/internal/workflow"
)

func TestPrepareCancellationDoesNotCommit(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := workflow.New(st)
	pkg, err := app.Create(workflow.CreateRequest{
		ProgramTitle:     "取消传播测试节目",
		EpisodeCode:      "CTX-01",
		AudioDurationMs:  5000,
		Language:         "zh-CN",
		DeliveryStandard: "WebVTT",
	}, "editor")
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents := len(st.Events(pkg.ID))

	body, err := json.Marshal(map[string]any{
		"expectedVersion": pkg.Version,
		"idempotencyKey":  "prepare-canceled-request",
		"cues": []domain.CaptionCue{{
			ID: "cue-ctx", Sequence: 1, StartMs: 0, EndMs: 1200,
			Speaker: "主持人", Text: "这条字幕不应被已取消的请求提交",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subtitle-packages/"+pkg.ID+"/prepare", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	httpapi.New(app).Handler().ServeHTTP(recorder, req)

	current, ok := st.Get(pkg.ID)
	if !ok {
		t.Fatal("取消请求后字幕包意外消失")
	}
	if current.Status != domain.StatusDraft || current.Version != pkg.Version || len(current.Cues) != 0 {
		t.Fatalf("已取消的 prepare 请求仍提交投影: status=%s version=%d cues=%d", current.Status, current.Version, len(current.Cues))
	}
	if got := len(st.Events(pkg.ID)); got != beforeEvents {
		t.Fatalf("已取消的 prepare 请求仍追加审计事件: before=%d after=%d", beforeEvents, got)
	}
	if recorder.Code >= 200 && recorder.Code < 300 {
		t.Fatalf("已取消的 prepare 请求仍返回成功状态: %d", recorder.Code)
	}
}
