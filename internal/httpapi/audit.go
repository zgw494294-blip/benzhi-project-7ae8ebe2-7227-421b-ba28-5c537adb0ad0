package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"subtitleqc/internal/store"
	"subtitleqc/internal/workflow"
	"time"
)

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/audit/"), "/"), "/")
	id := parts[0]
	if id == "" {
		fail(w, 400, fmt.Errorf("字幕包标识不能为空"))
		return
	}
	q := r.URL.Query()
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 200 {
			fail(w, 400, fmt.Errorf("limit 必须在0-200之间"))
			return
		}
		limit = n
	}
	parse := func(name string) (time.Time, error) {
		v := q.Get(name)
		if v == "" {
			return time.Time{}, nil
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s 日期无效", name)
		}
		return t, nil
	}
	from, err := parse("from")
	if err != nil {
		fail(w, 400, err)
		return
	}
	to, err := parse("to")
	if err != nil {
		fail(w, 400, err)
		return
	}
	events := store.FilterEvents(s.App.Store.Events(id), store.AuditQuery{PackageID: id, Type: q.Get("type"), Actor: q.Get("actor"), From: from, To: to, Limit: limit})
	writeJSON(w, 200, map[string]any{"events": events, "timeline": workflow.EventTimeline(events)})
}
