package httpapi

import (
	"fmt"
	"net/http"
	"strings"
)

func requireJSON(r *http.Request) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("请求必须使用 application/json")
	}
	return nil
}
func pathID(r *http.Request, prefix string) (string, error) {
	v := strings.TrimPrefix(r.URL.Path, prefix)
	v = strings.Trim(v, "/")
	if v == "" || strings.Contains(v, "/") {
		return "", fmt.Errorf("资源标识无效")
	}
	return v, nil
}
func parseExpected(v int) error {
	if v < 0 {
		return fmt.Errorf("expectedVersion 不能为负数")
	}
	return nil
}
func validateIdempotency(v string) error {
	if len(v) > 128 {
		return fmt.Errorf("idempotencyKey 过长")
	}
	return nil
}
func clientActor(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("X-Actor"))
	if v == "" {
		return "web-user"
	}
	if len(v) > 80 {
		return v[:80]
	}
	return v
}
