package snapshotfailure_test

import (
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/httpapi"
	"subtitleqc/internal/store"
	"subtitleqc/internal/workflow"
)

func TestSnapshotFailureDoesNotGhostCommit(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	app := workflow.New(st)
	pkg, err := app.Create(workflow.CreateRequest{
		ProgramTitle:     "快照故障测试节目",
		EpisodeCode:      "SNAP01",
		AudioDurationMs:  5000,
		Language:         "zh-CN",
		DeliveryStandard: "WebVTT WCAG",
	}, "editor")
	if err != nil {
		t.Fatal(err)
	}

	snapshotPath := st.SnapshotPath()
	oldSnapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents := len(st.Events(pkg.ID))
	if err := os.Remove(snapshotPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(snapshotPath, 0755); err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"expectedVersion":%d,"idempotencyKey":"snapshot-failure-prepare","cues":[{"id":"cue-snapshot","sequence":1,"startMs":0,"endMs":1000,"speaker":"主持人","text":"故障期间不能幽灵提交"}]}`, pkg.Version)
	req := httptest.NewRequest("POST", "/api/v1/subtitle-packages/"+pkg.ID+"/prepare", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	httpapi.New(app).Handler().ServeHTTP(recorder, req)

	current, ok := st.Get(pkg.ID)
	afterEvents := len(st.Events(pkg.ID))
	if err := os.Remove(snapshotPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, oldSnapshot, 0644); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, reopenErr := store.Open(dir)
	if reopened != nil {
		defer reopened.Close()
	}

	problems := make([]string, 0, 4)
	if recorder.Code >= 200 && recorder.Code < 300 {
		problems = append(problems, fmt.Sprintf("HTTP 返回成功状态 %d", recorder.Code))
	}
	if !ok || current.Status != domain.StatusDraft || current.Version != pkg.Version {
		problems = append(problems, fmt.Sprintf("内存投影变为 status=%s version=%d", current.Status, current.Version))
	}
	if afterEvents != beforeEvents {
		problems = append(problems, fmt.Sprintf("审计事件从 %d 增至 %d", beforeEvents, afterEvents))
	}
	if reopenErr != nil {
		problems = append(problems, "重启恢复失败: "+reopenErr.Error())
	} else if recovered, exists := reopened.Get(pkg.ID); !exists || recovered.Status != domain.StatusDraft || recovered.Version != pkg.Version {
		problems = append(problems, "重启后未恢复原 draft 投影")
	}
	if len(problems) > 0 {
		t.Fatalf("TestSnapshotFailureDoesNotGhostCommit: %s", strings.Join(problems, "; "))
	}
}
