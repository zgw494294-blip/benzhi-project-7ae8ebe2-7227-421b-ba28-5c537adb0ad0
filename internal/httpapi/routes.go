package httpapi

import "net/http"

func method(r *http.Request, allowed ...string) bool {
	for _, v := range allowed {
		if r.Method == v {
			return true
		}
	}
	return false
}
func withMethod(h http.HandlerFunc, allowed ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !method(r, allowed...) {
			respondError(w, http.StatusMethodNotAllowed, "method_not_allowed", "方法不允许")
			return
		}
		h(w, r)
	}
}
