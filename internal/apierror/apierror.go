package apierror

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Problem is an RFC 9457 application/problem+json response.
type Problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// Write writes an RFC 9457 problem response. It logs the full error server-side
// and returns only the sanitized Problem to the client (NIST SI-11, REC-API-23).
func Write(w http.ResponseWriter, r *http.Request, status int, title, detail string, err error) {
	if err != nil {
		slog.ErrorContext(r.Context(), title,
			"error", err.Error(),
			"status", status,
			"method", r.Method,
			"path", r.URL.Path,
		)
	}

	p := Problem{
		Type:     "about:blank",
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(p)
}

// BadRequest writes a 400 problem response.
func BadRequest(w http.ResponseWriter, r *http.Request, detail string, err error) {
	Write(w, r, http.StatusBadRequest, "Bad Request", detail, err)
}

// Unauthorized writes a 401 problem response.
func Unauthorized(w http.ResponseWriter, r *http.Request, detail string, err error) {
	Write(w, r, http.StatusUnauthorized, "Unauthorized", detail, err)
}

// Forbidden writes a 403 problem response.
func Forbidden(w http.ResponseWriter, r *http.Request, detail string, err error) {
	Write(w, r, http.StatusForbidden, "Forbidden", detail, err)
}

// NotFound writes a 404 problem response.
func NotFound(w http.ResponseWriter, r *http.Request, detail string, err error) {
	Write(w, r, http.StatusNotFound, "Not Found", detail, err)
}

// Conflict writes a 409 problem response.
func Conflict(w http.ResponseWriter, r *http.Request, detail string, err error) {
	Write(w, r, http.StatusConflict, "Conflict", detail, err)
}

// TooManyRequests writes a 429 problem response.
func TooManyRequests(w http.ResponseWriter, r *http.Request, detail string, err error) {
	Write(w, r, http.StatusTooManyRequests, "Too Many Requests", detail, err)
}

// Internal writes a 500 problem response. The detail is always generic —
// never expose internal error messages to clients.
func Internal(w http.ResponseWriter, r *http.Request, err error) {
	Write(w, r, http.StatusInternalServerError, "Internal Server Error",
		"An unexpected error occurred. Please try again later.", err)
}

// UnsupportedMediaType writes a 415 problem response.
func UnsupportedMediaType(w http.ResponseWriter, r *http.Request, detail string) {
	Write(w, r, http.StatusUnsupportedMediaType, "Unsupported Media Type", detail, nil)
}
