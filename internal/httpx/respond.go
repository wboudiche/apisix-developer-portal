package httpx

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"

	"apisix-portal/internal/i18n"
)

// JSON writes v as a JSON response with the given status code. It encodes into
// a buffer first so that an encoding error does not leave a partially-written
// response with an already-committed status code.
func JSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		log.Printf("httpx: JSON encode error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// Error writes a {"error": msg} body with the given status code.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// ErrorT writes a localized {"error": msg} body, resolving the message for the
// request's locale from the i18n catalog.
func ErrorT(w http.ResponseWriter, r *http.Request, status int, key string, args ...any) {
	Error(w, status, i18n.T(i18n.FromContext(r.Context()), key, args...))
}
