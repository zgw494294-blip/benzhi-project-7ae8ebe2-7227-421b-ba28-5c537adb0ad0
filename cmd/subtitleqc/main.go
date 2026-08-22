package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"subtitleqc/internal/httpapi"
	"subtitleqc/internal/store"
	"subtitleqc/internal/workflow"
)

func main() {
	addr := flag.String("addr", "", "监听地址")
	self := flag.Bool("selfcheck", false, "运行有界自检")
	data := flag.String("data-dir", "", "数据目录")
	flag.Parse()
	resolved := resolveAddr(*addr)
	if err := validateAddr(resolved); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *self {
		if err := selfcheck(resolved); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("自检通过")
		return
	}
	dir := *data
	if dir == "" {
		dir = filepath.Join(".", "data")
	}
	st, err := store.Open(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer st.Close()
	if err := st.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	srv := httpapi.New(workflow.New(st))
	go func() {
		if err := srv.ListenAndServe(resolved); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}()
	waitSignal()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
func resolveAddr(v string) string {
	if v != "" {
		if strings.HasPrefix(v, ":") {
			return "127.0.0.1" + v
		}
		return v
	}
	if p := os.Getenv("PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 && n < 65536 {
			return "127.0.0.1:" + p
		}
	}
	return "127.0.0.1:19137"
}
func selfcheck(addr string) error {
	dir, err := os.MkdirTemp("", "subtitleqc-selfcheck-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	st, err := store.Open(dir)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Validate(); err != nil {
		return err
	}
	srv := httpapi.New(workflow.New(st))
	go srv.ListenAndServe(addr)
	client := &http.Client{Timeout: 2 * time.Second}
	var base string
	for i := 0; i < 30; i++ {
		r, e := client.Get("http://" + addr + "/healthz")
		if e == nil {
			r.Body.Close()
			base = "http://" + addr
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if base == "" {
		return fmt.Errorf("服务未启动")
	}
	post := func(path string, v string) (int, error) {
		r, e := client.Post(base+path, "application/json", strings.NewReader(v))
		if e != nil {
			return 0, e
		}
		defer r.Body.Close()
		io.Copy(io.Discard, r.Body)
		return r.StatusCode, nil
	}
	status, e := post("/api/v1/subtitle-packages", `{"programTitle":"自检节目","episodeCode":"SMOKE","audioDurationMs":60000,"language":"zh-CN","deliveryStandard":"WebVTT"}`)
	if e != nil || status != 201 {
		return fmt.Errorf("建包失败: %d %v", status, e)
	}
	pkgs, e := client.Get(base + "/api/v1/subtitle-packages")
	if e != nil {
		return e
	}
	defer pkgs.Body.Close()
	var list struct {
		Packages []struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		} `json:"packages"`
	}
	if err := json.NewDecoder(pkgs.Body).Decode(&list); err != nil {
		return err
	}
	if len(list.Packages) != 1 {
		return fmt.Errorf("查询失败")
	}
	id := list.Packages[0].ID
	v := list.Packages[0].Version
	status, e = post("/api/v1/subtitle-packages/"+id+"/prepare", fmt.Sprintf(`{"expectedVersion":%d,"cues":[{"sequence":1,"startMs":0,"endMs":2000,"speaker":"播音员","text":"自检字幕","soundHint":"[音乐]"}]}`, v))
	if e != nil || status != 200 {
		return fmt.Errorf("准备失败: %d %v", status, e)
	}
	v++
	for _, step := range []string{"quality-check", "review"} {
		status, e = post("/api/v1/subtitle-packages/"+id+"/"+step, fmt.Sprintf(`{"expectedVersion":%d,"idempotencyKey":"selfcheck-%s"}`, v, step))
		if e != nil || status != 200 {
			return fmt.Errorf("%s 失败: %d %v", step, status, e)
		}
		v++
	}
	previewResponse, e := client.Get(base + "/api/v1/subtitle-packages/" + id + "/freeze-preview")
	if e != nil {
		return e
	}
	defer previewResponse.Body.Close()
	var preview struct {
		Preview struct {
			ExpectedVersion int    `json:"expectedVersion"`
			Checksum        string `json:"checksum"`
		} `json:"preview"`
	}
	if previewResponse.StatusCode != http.StatusOK || json.NewDecoder(previewResponse.Body).Decode(&preview) != nil {
		return fmt.Errorf("冻结预检失败: %d", previewResponse.StatusCode)
	}
	status, e = post("/api/v1/subtitle-packages/"+id+"/freeze", fmt.Sprintf(`{"expectedVersion":%d,"expectedChecksum":%q,"idempotencyKey":"selfcheck-freeze"}`, preview.Preview.ExpectedVersion, preview.Preview.Checksum))
	if e != nil || status != 200 {
		return fmt.Errorf("freeze 失败: %d %v", status, e)
	}
	v++
	status, e = post("/api/v1/subtitle-packages/"+id+"/deliver", fmt.Sprintf(`{"expectedVersion":%d,"idempotencyKey":"selfcheck-deliver"}`, v))
	if e != nil || status != 200 {
		return fmt.Errorf("deliver 失败: %d %v", status, e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
func waitSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
