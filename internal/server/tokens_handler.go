package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

var tokenNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 _\-]{0,98}[a-zA-Z0-9]$`)

// tokenResponse is the API response shape for a join token.
type tokenResponse struct {
	ID        string           `json:"id"`
	Type      string           `json:"type"`
	Name      string           `json:"name"`
	Labels    *json.RawMessage `json:"labels,omitempty"`
	UsedCount int              `json:"usedCount"`
	MaxUses   *int             `json:"maxUses,omitempty"`
	ExpiresAt *string          `json:"expiresAt,omitempty"`
	Revoked   bool             `json:"revoked"`
	RevokedAt *string          `json:"revokedAt,omitempty"`
	CreatedBy *string          `json:"createdBy,omitempty"`
	CreatedAt string           `json:"createdAt"`
}

func toTokenResponse(t db.JoinToken) tokenResponse {
	resp := tokenResponse{
		ID:        t.ID,
		Type:      t.Type,
		Name:      t.Name,
		UsedCount: t.UsedCount,
		MaxUses:   t.MaxUses,
		ExpiresAt: t.ExpiresAt,
		Revoked:   t.Revoked != 0,
		RevokedAt: t.RevokedAt,
		CreatedBy: t.CreatedBy,
		CreatedAt: t.CreatedAt,
	}
	if t.Labels != nil {
		raw := json.RawMessage(*t.Labels)
		resp.Labels = &raw
	}
	return resp
}

// tokenCreateResponse includes the plaintext token (shown once).
type tokenCreateResponse struct {
	tokenResponse
	Token string `json:"token"`
}

// handleListJoinTokens returns paginated join tokens.
// GET /api/v1/agents/tokens
func (s *Server) handleListJoinTokens(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	pp, err := parsePaginationParams(r)
	if err != nil {
		apierror.BadRequest(w, r, err.Error(), nil)
		return
	}

	q := r.URL.Query()
	tokens, err := s.db.ListJoinTokensPaginated(r.Context(), db.TokenListParams{
		TenantID: claims.TenantID,
		CursorAt: pp.CursorAt,
		CursorID: pp.CursorID,
		Limit:    pp.Limit,
		Type:     q.Get("type"),
		Revoked:  q.Get("revoked"),
		Search:   truncate(q.Get("search"), 500),
	})
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	hasMore := len(tokens) > pp.Limit
	if hasMore {
		tokens = tokens[:pp.Limit]
	}

	data := make([]tokenResponse, len(tokens))
	for i, t := range tokens {
		data[i] = toTokenResponse(t)
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

// createTokenRequest is the request body for POST /api/v1/agents/tokens.
type createTokenRequest struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Labels   map[string]string `json:"labels"`
	MaxUses  *int              `json:"maxUses"`
	TTLHours *int              `json:"ttlHours"`
}

// handleCreateJoinToken creates a new join token.
// POST /api/v1/agents/tokens
func (s *Server) handleCreateJoinToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	var req createTokenRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	// Validate name
	if req.Name == "" || len(req.Name) > 100 {
		apierror.BadRequest(w, r, "name is required (max 100 characters).", nil)
		return
	}
	if len(req.Name) >= 2 && !tokenNameRe.MatchString(req.Name) {
		apierror.BadRequest(w, r, "name must be alphanumeric with spaces, hyphens, and underscores.", nil)
		return
	}

	// Validate type
	if req.Type != "single_use" && req.Type != "persistent" {
		apierror.BadRequest(w, r, "type must be single_use or persistent.", nil)
		return
	}

	// Validate maxUses
	if req.MaxUses != nil && *req.MaxUses < 1 {
		apierror.BadRequest(w, r, "maxUses must be at least 1.", nil)
		return
	}
	if req.Type == "single_use" {
		one := 1
		req.MaxUses = &one
	}

	// Validate TTL
	if req.TTLHours != nil && (*req.TTLHours < 1 || *req.TTLHours > 8760) {
		apierror.BadRequest(w, r, "ttlHours must be between 1 and 8760.", nil)
		return
	}

	// Validate labels
	if len(req.Labels) > 50 {
		apierror.BadRequest(w, r, "Too many labels (max 50).", nil)
		return
	}

	// Generate token secret (32 bytes, CSPRNG)
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		apierror.Internal(w, r, fmt.Errorf("generating token secret: %w", err))
		return
	}
	plainToken := base64.RawURLEncoding.EncodeToString(secret)

	// HMAC-SHA256 hash for storage
	mac := hmac.New(sha256.New, []byte("conduit-join-token"))
	mac.Write(secret)
	tokenHash := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	ctx := r.Context()
	tokenID := uuid.NewString()

	var labelsJSON *string
	if len(req.Labels) > 0 {
		b, _ := json.Marshal(req.Labels)
		s := string(b)
		labelsJSON = &s
	}

	var expiresAt *string
	if req.TTLHours != nil {
		t := time.Now().UTC().Add(time.Duration(*req.TTLHours) * time.Hour).Format(time.RFC3339)
		expiresAt = &t
	}

	token := &db.JoinToken{
		ID:        tokenID,
		TenantID:  claims.TenantID,
		Type:      req.Type,
		Name:      req.Name,
		Labels:    labelsJSON,
		TokenHash: tokenHash,
		UsedCount: 0,
		MaxUses:   req.MaxUses,
		ExpiresAt: expiresAt,
		Revoked:   0,
		CreatedBy: &claims.Subject,
	}

	if err := s.db.CreateJoinToken(ctx, token); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "token.created",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"token_id":"` + tokenID + `","name":"` + req.Name + `","type":"` + req.Type + `"}`),
		Outcome:   "success",
	})

	resp := tokenCreateResponse{
		tokenResponse: toTokenResponse(*token),
		Token:         plainToken,
	}

	writeJSON(w, http.StatusCreated, resp)
}

// handleRevokeJoinToken revokes a join token.
// DELETE /api/v1/agents/tokens/{tokenId}
func (s *Server) handleRevokeJoinToken(w http.ResponseWriter, r *http.Request) {
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
	token, err := s.db.GetJoinTokenByID(ctx, tokenID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if token == nil || token.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Join token not found.", nil)
		return
	}

	if token.Revoked != 0 {
		apierror.Conflict(w, r, "Token is already revoked.", nil)
		return
	}

	if err := s.db.RevokeJoinToken(ctx, tokenID); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "token.revoked",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"token_id":"` + tokenID + `","name":"` + token.Name + `"}`),
		Outcome:   "success",
	})

	w.WriteHeader(http.StatusNoContent)
}
