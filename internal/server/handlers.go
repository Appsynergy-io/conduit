package server

import (
	"encoding/json"
	"net/http"

	"github.com/appsynergy-io/conduit/internal/apierror"
)

// handleHealth responds with server health status (public, no auth).
// GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// handleNotImplemented is a placeholder for endpoints not yet implemented.
func (s *Server) handleNotImplemented(w http.ResponseWriter, r *http.Request) {
	apierror.Write(w, r, http.StatusNotImplemented, "Not Implemented",
		"This endpoint is not yet implemented.", nil)
}
