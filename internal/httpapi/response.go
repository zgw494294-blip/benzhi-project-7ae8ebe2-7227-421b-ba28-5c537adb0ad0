package httpapi

import (
	"encoding/json"
	"net/http"
)

type Envelope struct {
	Data  any       `json:"data,omitempty"`
	Error *APIError `json:"error,omitempty"`
}
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: data})
}
func respondError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Error: &APIError{code, msg}})
}
