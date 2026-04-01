package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

// ciTokenResponse is the API response shape for a CI token (never includes the hash).
type ciTokenResponse struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  *string `json:"expiresAt"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt *string `json:"lastUsedAt"`
	CreatedBy  string  `json:"createdBy"`
}

func toCITokenResponse(t db.CIToken) ciTokenResponse {
	var scopes []string
	json.Unmarshal([]byte(t.Scopes), &scopes)
	if scopes == nil {
		scopes = []string{}
	}

	return ciTokenResponse{
		ID:         t.ID,
		Name:       t.Name,
		Scopes:     scopes,
		ExpiresAt:  t.ExpiresAt,
		CreatedAt:  t.CreatedAt,
		LastUsedAt: t.LastUsedAt,
		CreatedBy:  t.CreatedBy,
	}
}

// handleListCITokens returns paginated CI tokens for the tenant.
// GET /api/v1/auth/ci-tokens
func (s *Server) handleListCITokens(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	pp, err := parsePaginationParams(r)
	if err != nil {
		apierror.BadRequest(w, r, err.Error(), nil)
		return
	}

	tokens, err := s.db.ListCITokensPaginated(r.Context(), db.CITokenListParams{
		TenantID: claims.TenantID,
		CursorAt: pp.CursorAt,
		CursorID: pp.CursorID,
		Limit:    pp.Limit,
	})
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	hasMore := len(tokens) > pp.Limit
	if hasMore {
		tokens = tokens[:pp.Limit]
	}

	data := make([]ciTokenResponse, len(tokens))
	for i, t := range tokens {
		data[i] = toCITokenResponse(t)
	}

	pg := paginationMeta{HasMore: hasMore}
	if hasMore && len(tokens) > 0 {
		last := tokens[len(tokens)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		pg.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       data,
		"pagination": pg,
	})
}

// createCITokenRequest is the request body for POST /api/v1/auth/ci-tokens.
type createCITokenRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expiresAt"`
}

// handleCreateCIToken creates a new scoped CI token.
// POST /api/v1/auth/ci-tokens
func (s *Server) handleCreateCIToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	var req createCITokenRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	// Validate name (OWASP API4 — input length)
	if req.Name == "" || len(req.Name) > 255 {
		apierror.BadRequest(w, r, "name is required (max 255 characters).", nil)
		return
	}

	// Validate scopes (NIST IA-5 — least privilege)
	if msg := db.ValidateCITokenScopes(req.Scopes); msg != "" {
		apierror.BadRequest(w, r, msg, nil)
		return
	}

	// Validate expiresAt if provided
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			apierror.BadRequest(w, r, "expiresAt must be a valid RFC 3339 timestamp.", nil)
			return
		}
		if t.Before(time.Now().UTC()) {
			apierror.BadRequest(w, r, "expiresAt must be in the future.", nil)
			return
		}
	}

	// Generate token secret (32 bytes, CSPRNG — NIST SC-28)
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		apierror.Internal(w, r, err)
		return
	}
	plainToken := "cdci_" + base64.RawURLEncoding.EncodeToString(secret)

	// SHA-256 hash for storage (constant-time lookup not needed — hash is the index)
	h := sha256.Sum256([]byte(plainToken))
	tokenHash := base64.RawURLEncoding.EncodeToString(h[:])

	scopesJSON, _ := json.Marshal(req.Scopes)

	ctx := r.Context()
	tokenID := uuid.NewString()

	token := &db.CIToken{
		ID:        tokenID,
		TenantID:  claims.TenantID,
		CreatedBy: claims.Subject,
		Name:      req.Name,
		TokenHash: tokenHash,
		Scopes:    string(scopesJSON),
		ExpiresAt: req.ExpiresAt,
	}

	if err := s.db.CreateCIToken(ctx, token); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "ci_token.created",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"token_id":"` + tokenID + `","name":"` + token.Name + `"}`),
		Outcome:   "success",
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":        tokenID,
		"name":      token.Name,
		"token":     plainToken,
		"scopes":    req.Scopes,
		"expiresAt": req.ExpiresAt,
		"createdAt": token.CreatedAt,
	})
}

// handleRevokeCIToken permanently revokes a CI token.
// DELETE /api/v1/auth/ci-tokens/{tokenId}
func (s *Server) handleRevokeCIToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	tokenID := chi.URLParam(r, "tokenId")

	if _, err := uuid.Parse(tokenID); err != nil {
		apierror.BadRequest(w, r, "Invalid token ID format.", nil)
		return
	}

	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	ctx := r.Context()
	token, err := s.db.GetCITokenByID(ctx, tokenID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if token == nil || token.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "CI token not found.", nil)
		return
	}

	if err := s.db.DeleteCIToken(ctx, tokenID); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "ci_token.revoked",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"token_id":"` + tokenID + `","name":"` + token.Name + `"}`),
		Outcome:   "success",
	})

	w.WriteHeader(http.StatusNoContent)
}
