package workbench_report_stale_cache_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/httpapi"
	"subtitleqc/internal/store"
	"subtitleqc/internal/workflow"
)

func TestWorkbenchReportCacheTracksPackageVersion(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := workflow.New(st)
	p, err := app.Create(workflow.CreateRequest{
		ProgramTitle: "报告缓存测试节目", EpisodeCode: "REPORT-01", AudioDurationMs: 10000,
		Language: "zh-CN", DeliveryStandard: "WebVTT WCAG",
	}, "editor")
	if err != nil {
		t.Fatal(err)
	}

	handler := httpapi.New(app).Handler()
	detailPath := fmt.Sprintf("/api/v1/subtitle-packages/%s", p.ID)
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, detailPath, nil))
	if first.Code != http.StatusOK {
		t.Fatalf("首次详情请求返回 %d: %s", first.Code, first.Body.String())
	}

	p, err = app.Prepare(p.ID, workflow.CueRequest{
		ExpectedVersion: p.Version, IdempotencyKey: "report-prepare",
		Cues: []domain.CaptionCue{{ID: "cue-1", Sequence: 1, StartMs: 0, EndMs: 100, Text: "这是一条会稳定触发说话人缺失和阅读速度超限规则的字幕"}},
	}, "editor")
	if err != nil {
		t.Fatal(err)
	}
	p, err = app.Check(p.ID, p.Version, "report-check", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Findings) == 0 {
		t.Fatal("测试字幕未生成质检发现")
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, detailPath, nil))
	if second.Code != http.StatusOK {
		t.Fatalf("第二次详情请求返回 %d: %s", second.Code, second.Body.String())
	}
	var detail struct {
		Package   domain.SubtitlePackage `json:"package"`
		Checklist domain.Checklist       `json:"checklist"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Checklist.Total != len(detail.Package.Findings) {
		t.Fatalf("详情中的 package 有 %d 条发现，但缓存 checklist.total 仍为 %d", len(detail.Package.Findings), detail.Checklist.Total)
	}
}
