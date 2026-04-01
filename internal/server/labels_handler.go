package server

import (
	"net/http"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

// handleListLabels returns all label keys and values in use across agents.
// GET /api/v1/labels
func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	labels, err := s.db.ListLabels(r.Context(), claims.TenantID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	type labelEntry struct {
		Key    string   `json:"key"`
		Values []string `json:"values"`
		Count  int      `json:"count"`
	}

	data := make([]labelEntry, len(labels))
	for i, l := range labels {
		data[i] = labelEntry{
			Key:    l.Key,
			Values: l.Values,
			Count:  l.Count,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": data,
	})
}
