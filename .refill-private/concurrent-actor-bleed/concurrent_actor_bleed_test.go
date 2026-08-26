package concurrentactorbleed_test

import (
	"encoding/json"
	"io"
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

type gatedBody struct {
	reader  *strings.Reader
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (b *gatedBody) Read(p []byte) (int, error) {
	b.once.Do(func() {
		close(b.entered)
		<-b.release
	})
	return b.reader.Read(p)
}

func (b *gatedBody) Close() error { return nil }

func TestConcurrentRequestsKeepActorIsolation(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	handler := httpapi.New(workflow.New(st)).Handler()
	requestJSON := `{"programTitle":"并发节目","episodeCode":"EP-ACTOR","audioDurationMs":60000,"language":"zh-CN","deliveryStandard":"WebVTT"}`
	entered := make(chan struct{})
	release := make(chan struct{})
	firstBody := &gatedBody{reader: strings.NewReader(requestJSON), entered: entered, release: release}
	firstRequest := httptest.NewRequest(http.MethodPost, "/api/v1/subtitle-packages", nil)
	firstRequest.Header.Set("Content-Type", "application/json")
	firstRequest.Header.Set("X-Actor", "editor-a")
	firstRequest.Body = firstBody
	firstResponse := httptest.NewRecorder()
	firstDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(firstResponse, firstRequest)
		close(firstDone)
	}()

	<-entered
	secondRequest := httptest.NewRequest(http.MethodPost, "/api/v1/subtitle-packages", strings.NewReader(requestJSON))
	secondRequest.Header.Set("Content-Type", "application/json")
	secondRequest.Header.Set("X-Actor", "editor-b")
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, secondRequest)
	close(release)
	<-firstDone

	if firstResponse.Code != http.StatusCreated || secondResponse.Code != http.StatusCreated {
		t.Fatalf("创建请求失败: first=%d second=%d", firstResponse.Code, secondResponse.Code)
	}
	var created struct {
		Package domain.SubtitlePackage `json:"package"`
	}
	if err := json.NewDecoder(firstResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	events := st.Events(created.Package.ID)
	if len(events) != 1 {
		t.Fatalf("首个字幕包审计事件数=%d", len(events))
	}
	if events[0].Actor != "editor-a" {
		t.Fatalf("TestConcurrentRequestsKeepActorIsolation: 首个请求 actor=%q，期望 editor-a", events[0].Actor)
	}
}

var _ io.ReadCloser = (*gatedBody)(nil)
