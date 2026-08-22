package httpapi

import (
	"net/http"
	"time"
)

type Health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Time    string `json:"time"`
}

func healthPayload() Health {
	return Health{Status: "ok", Version: "1.0.0", Time: time.Now().UTC().Format(time.RFC3339)}
}
func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, 405, "method_not_allowed", "方法不允许")
		return
	}
	respond(w, 200, healthPayload())
}
