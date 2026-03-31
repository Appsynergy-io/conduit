package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/db"
)

const (
	webhookDeliveryTimeout = 10 * time.Second
	webhookMaxRetries      = 3
	webhookQueueSize       = 1024
)

// WebhookPayload is the JSON body sent to webhook subscribers.
type WebhookPayload struct {
	ID        string      `json:"id"`        // Delivery ID
	EventType string      `json:"eventType"` // e.g. "agent.connected"
	TenantID  string      `json:"tenantId"`
	Timestamp string      `json:"timestamp"` // UTC RFC3339
	Data      interface{} `json:"data"`
}

// webhookWork is an internal work item for the delivery queue.
type webhookWork struct {
	sub       db.WebhookSubscription
	payload   WebhookPayload
	attempt   int
	retryAt   time.Time
}

// WebhookDeliverer delivers webhook events to subscribers asynchronously.
// It maintains a background worker that processes delivery jobs from a queue.
type WebhookDeliverer struct {
	database *db.DB
	logger   *slog.Logger
	client   *http.Client
	queue    chan webhookWork
	wg       sync.WaitGroup
	cancel   context.CancelFunc
}

// NewWebhookDeliverer creates a webhook delivery engine.
func NewWebhookDeliverer(database *db.DB, logger *slog.Logger) *WebhookDeliverer {
	return &WebhookDeliverer{
		database: database,
		logger:   logger,
		client: &http.Client{
			Timeout: webhookDeliveryTimeout,
			// Disable redirects (OWASP A10, NIST API7)
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		queue: make(chan webhookWork, webhookQueueSize),
	}
}

// Start begins the background delivery workers.
func (wd *WebhookDeliverer) Start(ctx context.Context) {
	ctx, wd.cancel = context.WithCancel(ctx)

	// Run 4 concurrent delivery workers
	for i := 0; i < 4; i++ {
		wd.wg.Add(1)
		go wd.worker(ctx)
	}
}

// Stop gracefully shuts down the delivery workers.
func (wd *WebhookDeliverer) Stop() {
	if wd.cancel != nil {
		wd.cancel()
	}
	close(wd.queue)
	wd.wg.Wait()
}

// Enqueue dispatches a webhook event to all matching subscribers for the tenant.
// This method is safe for concurrent use and returns immediately.
func (wd *WebhookDeliverer) Enqueue(ctx context.Context, tenantID, eventType string, data interface{}) {
	subs, err := wd.database.ListEnabledWebhooksByEvent(ctx, tenantID, eventType)
	if err != nil {
		wd.logger.Error("failed to list webhook subscribers", "error", err, "event", eventType)
		return
	}

	if len(subs) == 0 {
		return
	}

	payload := WebhookPayload{
		EventType: eventType,
		TenantID:  tenantID,
		Timestamp: db.Now(),
		Data:      data,
	}

	for _, sub := range subs {
		payload.ID = uuid.NewString() // Unique per delivery
		work := webhookWork{
			sub:     sub,
			payload: payload,
			attempt: 1,
		}

		select {
		case wd.queue <- work:
		default:
			wd.logger.Warn("webhook delivery queue full, dropping event",
				"event", eventType, "subscription_id", sub.ID)
		}
	}
}

// worker processes webhook deliveries from the queue.
func (wd *WebhookDeliverer) worker(ctx context.Context) {
	defer wd.wg.Done()

	for {
		select {
		case work, ok := <-wd.queue:
			if !ok {
				return
			}

			// Wait for retry time if needed
			if !work.retryAt.IsZero() && time.Now().Before(work.retryAt) {
				select {
				case <-time.After(time.Until(work.retryAt)):
				case <-ctx.Done():
					return
				}
			}

			wd.deliver(ctx, work)

		case <-ctx.Done():
			return
		}
	}
}

// deliver sends a single webhook payload to the subscriber.
func (wd *WebhookDeliverer) deliver(ctx context.Context, work webhookWork) {
	body, err := json.Marshal(work.payload)
	if err != nil {
		wd.logger.Error("failed to marshal webhook payload", "error", err)
		return
	}

	// Sign the payload with HMAC-SHA256
	signature := signPayload(work.sub.SecretHash, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, work.sub.URL, bytes.NewReader(body))
	if err != nil {
		wd.logger.Error("failed to create webhook request", "error", err, "url", work.sub.URL)
		wd.recordDelivery(ctx, work, "failed", nil, 0)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Conduit-Webhook/1.0")
	req.Header.Set("X-Conduit-Signature", signature)
	req.Header.Set("X-Conduit-Event", work.payload.EventType)
	req.Header.Set("X-Conduit-Delivery", work.payload.ID)

	start := time.Now()
	resp, err := wd.client.Do(req)
	elapsed := int(time.Since(start).Milliseconds())

	if err != nil {
		wd.logger.Warn("webhook delivery failed",
			"error", err,
			"url", work.sub.URL,
			"event", work.payload.EventType,
			"attempt", work.attempt,
		)
		wd.recordDelivery(ctx, work, "failed", nil, elapsed)
		wd.maybeRetry(work)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024)) // drain response

	httpStatus := resp.StatusCode

	if httpStatus >= 200 && httpStatus < 300 {
		wd.recordDelivery(ctx, work, "success", &httpStatus, elapsed)
		wd.logger.Debug("webhook delivered",
			"url", work.sub.URL,
			"event", work.payload.EventType,
			"status", httpStatus,
			"ms", elapsed,
		)
	} else {
		wd.recordDelivery(ctx, work, "failed", &httpStatus, elapsed)
		wd.logger.Warn("webhook delivery rejected",
			"url", work.sub.URL,
			"event", work.payload.EventType,
			"status", httpStatus,
			"attempt", work.attempt,
		)
		wd.maybeRetry(work)
	}
}

// maybeRetry re-enqueues a failed delivery with exponential backoff.
func (wd *WebhookDeliverer) maybeRetry(work webhookWork) {
	if work.attempt >= webhookMaxRetries {
		wd.logger.Warn("webhook delivery exhausted retries",
			"url", work.sub.URL,
			"event", work.payload.EventType,
		)
		return
	}

	// Exponential backoff: 5s, 25s, 125s
	delay := time.Duration(5) * time.Second
	for i := 1; i < work.attempt; i++ {
		delay *= 5
	}

	work.attempt++
	work.retryAt = time.Now().Add(delay)

	select {
	case wd.queue <- work:
	default:
		wd.logger.Warn("webhook retry queue full, dropping retry",
			"event", work.payload.EventType,
			"subscription_id", work.sub.ID,
		)
	}
}

// recordDelivery persists a webhook delivery record in the database.
func (wd *WebhookDeliverer) recordDelivery(ctx context.Context, work webhookWork, status string, httpStatus *int, responseTime int) {
	delivery := &db.WebhookDelivery{
		ID:             uuid.NewString(),
		TenantID:       work.sub.TenantID,
		SubscriptionID: work.sub.ID,
		EventType:      work.payload.EventType,
		Status:         status,
		HTTPStatus:     httpStatus,
		ResponseTime:   &responseTime,
		AttemptNumber:  work.attempt,
		AttemptedAt:    db.Now(),
	}

	if status == "success" {
		now := db.Now()
		delivery.DeliveredAt = &now
	}

	if err := wd.database.CreateWebhookDelivery(ctx, delivery); err != nil {
		wd.logger.Error("failed to record webhook delivery", "error", err)
	}
}

// signPayload computes an HMAC-SHA256 signature for the webhook payload.
// Returns the hex-encoded signature string.
func signPayload(secretHash string, body []byte) string {
	keyBytes, err := hex.DecodeString(secretHash)
	if err != nil {
		// Fall back to using the hash string as key bytes
		keyBytes = []byte(secretHash)
	}

	mac := hmac.New(sha256.New, keyBytes)
	mac.Write(body)
	return fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))
}
