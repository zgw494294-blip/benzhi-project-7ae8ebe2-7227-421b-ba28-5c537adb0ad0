package httpapi

import (
	"net/http"
	"strings"
)

func requestID(r *http.Request) string {
	if v := r.Header.Get("X-Request-ID"); v != "" {
		return v
	}
	return "generated-request"
}
func role(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("X-Role"))
	if v == "" {
		return "editor"
	}
	return v
}
func contentTypeJSON(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
}
