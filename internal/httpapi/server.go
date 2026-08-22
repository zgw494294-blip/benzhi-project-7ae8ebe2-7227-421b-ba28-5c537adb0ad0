package httpapi

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/store"
	"subtitleqc/internal/webui"
	"subtitleqc/internal/workflow"
)

type Server struct {
	App  *workflow.Service
	mux  *http.ServeMux
	http *http.Server
}

func New(app *workflow.Service) *Server {
	s := &Server{App: app, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return security(s.mux) }
func (s *Server) ListenAndServe(addr string) error {
	s.http = &http.Server{Addr: addr, Handler: s.Handler(), ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	return s.http.ListenAndServe()
}
func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}
func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.health)
	s.mux.HandleFunc("/", s.index)
	s.mux.HandleFunc("/assets/", s.asset)
	s.mux.HandleFunc("/api/v1/subtitle-packages", s.packages)
	s.mux.HandleFunc("/api/v1/subtitle-packages/", s.packageAction)
	s.mux.HandleFunc("/api/v1/audit/", s.audit)
	s.mux.HandleFunc("/api/v1/credentials", s.credentials)
	s.mux.HandleFunc("/api/v1/statistics/quality", s.qualityStatistics)
}
func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok"})
}
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	f, _ := webui.Assets.ReadFile("index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(f)
}
func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/assets/")
	f, err := webui.Assets.ReadFile(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension("."+strings.Split(name, ".")[1]))
	_, _ = w.Write(f)
}
func (s *Server) packages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q, err := packageQuery(r)
		if err != nil {
			fail(w, 400, err)
			return
		}
		result := s.App.QueryPackages(q)
		packages := result.Packages
		counts := result.Counts
		stats := map[string]any{"total": 0, "delivered": 0, "byStatus": map[string]int{}}
		by := stats["byStatus"].(map[string]int)
		for _, st := range []domain.Status{domain.StatusDraft, domain.StatusPrepared, domain.StatusReviewing, domain.StatusCorrectionRequired, domain.StatusReviewPassed, domain.StatusFrozen, domain.StatusDelivered} {
			by[string(st)] = counts[st]
			stats["total"] = stats["total"].(int) + counts[st]
		}
		stats["delivered"] = counts[domain.StatusDelivered]
		writeJSON(w, 200, map[string]any{"packages": packages, "stats": stats, "metrics": result.Metrics, "limit": result.Limit})
	case http.MethodPost:
		var req workflow.CreateRequest
		if !decode(w, r, &req) {
			return
		}
		p, err := s.App.Create(req, actor(r))
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 201, map[string]any{"package": p})
	default:
		methodNotAllowed(w)
	}
}
func (s *Server) packageAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/subtitle-packages/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		p, ok := s.App.Store.Get(id)
		if !ok {
			fail(w, 404, errors.New("字幕包不存在"))
			return
		}
		proj, _ := s.App.Report(id)
		writeJSON(w, 200, map[string]any{"package": p, "events": s.App.Store.Events(id), "checklist": proj.Checklist, "timeline": workflow.EventTimeline(s.App.Store.Events(id)), "summary": proj.Summary})
		return
	}
	if len(parts) == 2 && r.Method == http.MethodGet && parts[1] == "credential" {
		p, ok := s.App.Store.Get(id)
		if !ok {
			fail(w, 404, errors.New("字幕包不存在"))
			return
		}
		if p.Credential == nil {
			fail(w, 404, errors.New("凭据尚未签发"))
			return
		}
		if p.Master == nil || p.Credential.MasterChecksum != p.Master.Checksum {
			fail(w, 409, errors.New("凭据完整性校验失败"))
			return
		}
		writeJSON(w, 200, map[string]any{"credential": p.Credential})
		return
	}
	if len(parts) == 2 && r.Method == http.MethodGet && parts[1] == "findings" {
		p, ok := s.App.Store.Get(id)
		if !ok {
			fail(w, 404, errors.New("字幕包不存在"))
			return
		}
		if p.Status == domain.StatusDraft || p.Status == domain.StatusPrepared {
			fail(w, 409, errors.New("字幕包尚未质检"))
			return
		}
		writeJSON(w, 200, map[string]any{"findings": p.Findings, "checklist": domain.BuildChecklist(p)})
		return
	}
	if len(parts) == 2 && r.Method == http.MethodGet && parts[1] == "master" {
		p, ok := s.App.Store.Get(id)
		if !ok {
			fail(w, 404, errors.New("字幕包不存在"))
			return
		}
		if p.Master == nil {
			fail(w, 404, errors.New("冻结母版不存在"))
			return
		}
		_, _, checksum := domain.WebVTT(p)
		if checksum != p.Master.Checksum || domain.ContentChecksum(p.Master.Content) != p.Master.Checksum {
			fail(w, 409, errors.New("冻结母版完整性校验失败"))
			return
		}
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.vtt"`)
		_, _ = w.Write([]byte(p.Master.Content))
		return
	}
	if len(parts) == 2 && r.Method == http.MethodGet && parts[1] == "freeze-preview" {
		preview, err := s.App.PreviewFreeze(id)
		if err != nil {
			fail(w, 409, err)
			return
		}
		writeJSON(w, 200, map[string]any{"preview": preview})
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	if action == "manual-findings" {
		var req struct {
			ExpectedVersion int    `json:"expectedVersion"`
			IdempotencyKey  string `json:"idempotencyKey"`
			Role            string `json:"role"`
			CueID           string `json:"cueId"`
			StartMs         *int64 `json:"startMs"`
			EndMs           *int64 `json:"endMs"`
			RuleCode        string `json:"ruleCode"`
			Severity        string `json:"severity"`
			Message         string `json:"message"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Role == "" {
			req.Role = strings.TrimSpace(r.Header.Get("X-Role"))
		}
		p, err := s.App.AddManualFinding(id, workflow.ManualFindingRequest{
			ExpectedVersion: req.ExpectedVersion, IdempotencyKey: req.IdempotencyKey, Role: req.Role,
			CueID: req.CueID, StartMs: req.StartMs, EndMs: req.EndMs, RuleCode: req.RuleCode,
			Severity: req.Severity, Message: req.Message,
		}, actor(r))
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"package": p})
		return
	}
	if action == "revisions" && len(parts) == 3 && parts[2] == "preview" {
		var req struct {
			Changes    []domain.RevisionChange `json:"changes"`
			FindingIDs []string                `json:"findingIds"`
		}
		if !decode(w, r, &req) {
			return
		}
		preview, err := s.App.PreviewRevision(id, workflow.RevisionPreviewRequest{Changes: req.Changes, FindingIDs: req.FindingIDs})
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"preview": preview})
		return
	}
	var body struct {
		ExpectedVersion  int                           `json:"expectedVersion"`
		IdempotencyKey   string                        `json:"idempotencyKey"`
		Reason           string                        `json:"reason"`
		FindingID        string                        `json:"findingId"`
		Disposition      string                        `json:"disposition"`
		ResolutionNote   string                        `json:"resolutionNote"`
		Cues             []domain.CaptionCue           `json:"cues"`
		Changes          []domain.RevisionChange       `json:"changes"`
		FindingIDs       []string                      `json:"findingIds"`
		ExpectedChecksum string                        `json:"expectedChecksum"`
		Role             string                        `json:"role"`
		Findings         []workflow.FindingDisposition `json:"findings"`
		Raw              string                        `json:"raw"`
		Content          string                        `json:"content"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Role == "" {
		body.Role = strings.TrimSpace(r.Header.Get("X-Role"))
	}
	if body.Role != "" {
		actionRole := action
		if action == "quality-check" {
			actionRole = "check"
		}
		if action == "findings" || action == "finding-batch" {
			actionRole = "finding"
		}
		if !workflowRoleAllows(body.Role, actionRole) {
			fail(w, 403, fmt.Errorf("角色%s无权执行%s", body.Role, action))
			return
		}
	}
	var p *domain.SubtitlePackage
	var err error
	switch action {
	case "prepare-preview", "import-preview":
		raw := body.Raw
		if raw == "" {
			raw = body.Content
		}
		cues, e := s.App.PreviewImport(id, raw)
		if e != nil {
			fail(w, 400, e)
			return
		}
		writeJSON(w, 200, map[string]any{"cues": cues, "packageId": id})
		return
	case "prepare":
		if len(body.Cues) == 0 && body.Raw != "" {
			body.Cues, err = s.App.PreviewImport(id, body.Raw)
		}
		if err != nil {
			fail(w, 400, err)
			return
		}
		p, err = s.App.Prepare(id, workflow.CueRequest{ExpectedVersion: body.ExpectedVersion, IdempotencyKey: body.IdempotencyKey, Cues: body.Cues}, actor(r))
	case "quality-check":
		p, err = s.App.Check(id, body.ExpectedVersion, body.IdempotencyKey, actor(r))
	case "finding":
		p, err = s.App.Disposition(id, body.FindingID, body.Disposition, body.ResolutionNote, actor(r))
	case "findings", "finding-batch":
		p, err = s.App.BatchDisposition(id, body.ExpectedVersion, body.IdempotencyKey, actor(r), body.Role, body.Findings)
	case "review":
		p, err = s.App.SubmitReview(id, body.ExpectedVersion, body.IdempotencyKey, actor(r))
	case "revisions":
		p, err = s.App.Revise(id, body.ExpectedVersion, body.IdempotencyKey, body.Reason, actor(r), body.Changes, body.FindingIDs)
	case "freeze":
		p, err = s.App.FreezeConfirmed(id, body.ExpectedVersion, body.ExpectedChecksum, body.IdempotencyKey, actor(r))
	case "deliver":
		p, err = s.App.Deliver(id, body.ExpectedVersion, body.IdempotencyKey, actor(r))
	case "credential":
		p, ok := s.App.Store.Get(id)
		if !ok {
			err = errors.New("字幕包不存在")
		} else if p.Credential == nil {
			err = errors.New("凭据尚未签发")
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"package": p})
}

func workflowRoleAllows(role, action string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		return true
	case "editor":
		return action == "prepare" || action == "revisions"
	case "reviewer":
		return action == "quality-check" || action == "check" || action == "finding" || action == "review"
	case "delivery":
		return action == "freeze" || action == "deliver"
	}
	return false
}

func packageQuery(r *http.Request) (store.PackageQuery, error) {
	status := domain.Status(strings.TrimSpace(r.URL.Query().Get("status")))
	if status != "" {
		switch status {
		case domain.StatusDraft, domain.StatusPrepared, domain.StatusReviewing, domain.StatusCorrectionRequired, domain.StatusReviewPassed, domain.StatusFrozen, domain.StatusDelivered:
		default:
			return store.PackageQuery{}, fmt.Errorf("status 无效")
		}
	}
	limit := store.DefaultPackageLimit
	limitSet := true
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limitSet = true
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > store.MaxPackageLimit {
			return store.PackageQuery{}, fmt.Errorf("limit 必须在0-%d之间", store.MaxPackageLimit)
		}
		limit = n
	}
	textQuery := r.URL.Query().Get("q")
	if textQuery == "" {
		textQuery = r.URL.Query().Get("text")
	}
	return store.PackageQuery{Status: status, Text: textQuery, RuleCode: strings.TrimSpace(r.URL.Query().Get("ruleCode")), Limit: store.NormalizePackageLimit(limit), LimitSet: limitSet}, nil
}

func (s *Server) credentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	credentials, err := s.App.Credentials()
	if err != nil {
		fail(w, 409, err)
		return
	}
	writeJSON(w, 200, map[string]any{"credentials": credentials})
}
func actor(r *http.Request) string {
	if v := r.Header.Get("X-Actor"); v != "" {
		return v
	}
	return "web-user"
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil && err != io.EOF {
		fail(w, 400, errors.New("请求 JSON 无效"))
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		fail(w, 400, errors.New("请求 JSON 只能包含一个对象"))
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, POST")
	fail(w, 405, errors.New("方法不允许"))
}

var _ embed.FS
var _ = strconv.Itoa
