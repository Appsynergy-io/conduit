package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

// webhookResponse is the API response shape for a webhook subscription.
type webhookResponse struct {
	ID        string          `json:"id"`
	URL       string          `json:"url"`
	Events    json.RawMessage `json:"events"`
	Enabled   bool            `json:"enabled"`
	CreatedBy *string         `json:"createdBy,omitempty"`
	CreatedAt string          `json:"createdAt"`
}

func toWebhookResponse(sub db.WebhookSubscription) webhookResponse {
	return webhookResponse{
		ID:        sub.ID,
		URL:       sub.URL,
		Events:    json.RawMessage(sub.Events),
		Enabled:   sub.Enabled,
		CreatedBy: sub.CreatedBy,
		CreatedAt: sub.CreatedAt,
	}
}

// webhookCreateResponse includes the signing secret (shown once).
type webhookCreateResponse struct {
	webhookResponse
	Secret string `json:"secret"`
}

// handleListWebhooks returns paginated webhook subscriptions.
// GET /api/v1/webhooks
func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	pp, err := parsePaginationParams(r)
	if err != nil {
		apierror.BadRequest(w, r, err.Error(), nil)
		return
	}

	q := r.URL.Query()
	subs, err := s.db.ListWebhooksPaginated(r.Context(), db.WebhookListParams{
		TenantID: claims.TenantID,
		CursorAt: pp.CursorAt,
		CursorID: pp.CursorID,
		Limit:    pp.Limit,
		Enabled:  q.Get("enabled"),
		Search:   truncate(q.Get("search"), 500),
	})
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	hasMore := len(subs) > pp.Limit
	if hasMore {
		subs = subs[:pp.Limit]
	}

	data := make([]webhookResponse, len(subs))
	for i, sub := range subs {
		data[i] = toWebhookResponse(sub)
	}

	pg := paginationMeta{HasMore: hasMore}
	if hasMore && len(subs) > 0 {
		last := subs[len(subs)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		pg.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       data,
		"pagination": pg,
	})
}

// Valid webhook event types.
var validWebhookEvents = map[string]bool{
	"agent.connected":    true,
	"agent.disconnected": true,
	"agent.deleted":      true,
	"auth.login":         true,
	"auth.logout":        true,
	"user.created":       true,
	"user.updated":       true,
	"session.revoked":    true,
	"shell.start":        true,
	"shell.end":          true,
	"file.download":      true,
	"file.upload":        true,
	"exec.start":         true,
	"exec.complete":      true,
}

// createWebhookRequest is the request body for POST /api/v1/webhooks.
type createWebhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

// handleCreateWebhook creates a new webhook subscription.
// POST /api/v1/webhooks
func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	var req createWebhookRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	// Validate URL
	if req.URL == "" {
		apierror.BadRequest(w, r, "url is required.", nil)
		return
	}
	if len(req.URL) > 2048 {
		apierror.BadRequest(w, r, "url too long (max 2048 characters).", nil)
		return
	}

	parsedURL, err := url.Parse(req.URL)
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") {
		apierror.BadRequest(w, r, "url must be a valid HTTPS URL.", nil)
		return
	}

	// In production, only HTTPS is allowed.
	// In dev mode, http://localhost and http://127.0.0.1 are permitted.
	if parsedURL.Scheme == "http" {
		host := strings.Split(parsedURL.Host, ":")[0]
		if s.cfg.Server.Mode != "dev" || (host != "localhost" && host != "127.0.0.1") {
			apierror.BadRequest(w, r, "url must use HTTPS in production. HTTP is only allowed for localhost in dev mode.", nil)
			return
		}
	}

	// Validate events
	if len(req.Events) == 0 {
		apierror.BadRequest(w, r, "At least one event type is required.", nil)
		return
	}
	if len(req.Events) > 50 {
		apierror.BadRequest(w, r, "Too many event types (max 50).", nil)
		return
	}
	for _, e := range req.Events {
		if !validWebhookEvents[e] {
			apierror.BadRequest(w, r, "Invalid event type: "+e, nil)
			return
		}
	}

	// Generate signing secret (32 bytes, CSPRNG)
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		apierror.Internal(w, r, fmt.Errorf("generating webhook secret: %w", err))
		return
	}
	plainSecret := base64.RawURLEncoding.EncodeToString(secretBytes)

	// Hash for storage
	secretHash := fmt.Sprintf("%x", secretBytes)

	ctx := r.Context()
	subID := uuid.NewString()

	eventsJSON, _ := json.Marshal(req.Events)

	sub := &db.WebhookSubscription{
		ID:         subID,
		TenantID:   claims.TenantID,
		URL:        req.URL,
		SecretHash: secretHash,
		Events:     string(eventsJSON),
		Enabled:    true,
		CreatedBy:  &claims.Subject,
	}

	if err := s.db.CreateWebhookSubscription(ctx, sub); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "webhook.created",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"webhook_id":"` + subID + `","url":"` + req.URL + `"}`),
		Outcome:   "success",
	})

	resp := webhookCreateResponse{
		webhookResponse: toWebhookResponse(*sub),
		Secret:          plainSecret,
	}

	writeJSON(w, http.StatusCreated, resp)
}
