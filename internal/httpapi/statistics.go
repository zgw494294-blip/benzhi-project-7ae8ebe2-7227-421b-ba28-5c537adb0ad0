package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"subtitleqc/internal/workflow"
)

func (s *Server) qualityStatistics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	q := r.URL.Query()
	from, err := parseStatisticsTime("from", q.Get("from"), false)
	if err != nil {
		fail(w, 400, err)
		return
	}
	to, err := parseStatisticsTime("to", q.Get("to"), true)
	if err != nil {
		fail(w, 400, err)
		return
	}
	if !from.IsZero() && !to.IsZero() {
		if from.After(to) {
			fail(w, 400, fmt.Errorf("from 不能晚于 to"))
			return
		}
		if to.Sub(from) > 366*24*time.Hour {
			fail(w, 400, fmt.Errorf("查询时间范围不能超过366天"))
			return
		}
	}
	result := s.App.QualityStatistics(workflow.StatisticsFilter{
		From: from, To: to, Language: strings.TrimSpace(q.Get("language")), DeliveryStandard: strings.TrimSpace(q.Get("deliveryStandard")),
	})
	writeJSON(w, 200, map[string]any{"statistics": result})
}

func parseStatisticsTime(name, value string, endOfDay bool) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s 日期无效", name)
	}
	if endOfDay {
		parsed = parsed.Add(24*time.Hour - time.Nanosecond)
	}
	return parsed, nil
}
