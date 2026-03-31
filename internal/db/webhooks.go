package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// WebhookSubscription represents a row from the webhook_subscriptions table.
type WebhookSubscription struct {
	ID         string
	TenantID   string
	URL        string
	SecretHash string
	Events     string // JSON array of event types
	Enabled    bool
	CreatedBy  *string
	CreatedAt  string
}

// WebhookDelivery represents a row from the webhook_deliveries table.
type WebhookDelivery struct {
	ID             string
	TenantID       string
	SubscriptionID string
	EventType      string
	Status         string
	HTTPStatus     *int
	ResponseTime   *int
	AttemptNumber  int
	NextRetryAt    *string
	DeliveredAt    *string
	AttemptedAt    string
}

// CreateWebhookSubscription inserts a new webhook subscription.
func (d *DB) CreateWebhookSubscription(ctx context.Context, sub *WebhookSubscription) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO webhook_subscriptions
			(id, tenant_id, url, secret_hash, events, enabled, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.ID, sub.TenantID, sub.URL, sub.SecretHash, sub.Events, sub.Enabled, sub.CreatedBy, Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting webhook subscription: %w", err)
	}
	return nil
}

// GetWebhookSubscriptionByID returns a single webhook subscription by ID.
func (d *DB) GetWebhookSubscriptionByID(ctx context.Context, id string) (*WebhookSubscription, error) {
	var sub WebhookSubscription
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, tenant_id, url, secret_hash, events, enabled, created_by, created_at
		FROM webhook_subscriptions WHERE id = ?`, id,
	).Scan(&sub.ID, &sub.TenantID, &sub.URL, &sub.SecretHash, &sub.Events, &sub.Enabled, &sub.CreatedBy, &sub.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying webhook subscription: %w", err)
	}
	return &sub, nil
}

// ListWebhookSubscriptions returns all webhook subscriptions for the given tenant,
// ordered by created_at ascending.
func (d *DB) ListWebhookSubscriptions(ctx context.Context, tenantID string) ([]WebhookSubscription, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, tenant_id, url, secret_hash, events, enabled, created_by, created_at
		FROM webhook_subscriptions WHERE tenant_id = ?
		ORDER BY created_at`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing webhook subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []WebhookSubscription
	for rows.Next() {
		var sub WebhookSubscription
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.URL, &sub.SecretHash, &sub.Events, &sub.Enabled, &sub.CreatedBy, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning webhook subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// WebhookListParams holds filters for paginated webhook subscription listing.
type WebhookListParams struct {
	TenantID string
	CursorAt string
	CursorID string
	Limit    int
	Enabled  string // "true" or "false"
	Search   string
}

// ListWebhooksPaginated returns webhook subscriptions with cursor-based pagination.
func (d *DB) ListWebhooksPaginated(ctx context.Context, p WebhookListParams) ([]WebhookSubscription, error) {
	query := `SELECT id, tenant_id, url, secret_hash, events, enabled, created_by, created_at
	           FROM webhook_subscriptions WHERE tenant_id = ?`
	args := []interface{}{p.TenantID}

	if p.Enabled == "true" {
		query += ` AND enabled = 1`
	} else if p.Enabled == "false" {
		query += ` AND enabled = 0`
	}
	if p.Search != "" {
		query += ` AND url LIKE ?`
		term := "%" + strings.ReplaceAll(p.Search, "%", "") + "%"
		args = append(args, term)
	}
	if p.CursorAt != "" {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, p.CursorAt, p.CursorAt, p.CursorID)
	}

	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, p.Limit+1)

	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing webhooks paginated: %w", err)
	}
	defer rows.Close()

	var subs []WebhookSubscription
	for rows.Next() {
		var sub WebhookSubscription
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.URL, &sub.SecretHash, &sub.Events, &sub.Enabled, &sub.CreatedBy, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning webhook subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// UpdateWebhookSubscription updates the url, events, and enabled flag on a subscription.
func (d *DB) UpdateWebhookSubscription(ctx context.Context, id, url, events string, enabled bool) error {
	result, err := d.conn.ExecContext(ctx,
		`UPDATE webhook_subscriptions SET url = ?, events = ?, enabled = ? WHERE id = ?`,
		url, events, enabled, id,
	)
	if err != nil {
		return fmt.Errorf("updating webhook subscription: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("webhook subscription not found: %s", id)
	}
	return nil
}

// DeleteWebhookSubscription removes a webhook subscription by ID.
func (d *DB) DeleteWebhookSubscription(ctx context.Context, id string) error {
	result, err := d.conn.ExecContext(ctx,
		`DELETE FROM webhook_subscriptions WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("deleting webhook subscription: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("webhook subscription not found: %s", id)
	}
	return nil
}

// ListEnabledWebhooksByEvent returns all enabled webhook subscriptions for the given
// tenant whose events JSON array contains the specified event type.
func (d *DB) ListEnabledWebhooksByEvent(ctx context.Context, tenantID, eventType string) ([]WebhookSubscription, error) {
	pattern := fmt.Sprintf("%%\"%s\"%%", eventType)
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, tenant_id, url, secret_hash, events, enabled, created_by, created_at
		FROM webhook_subscriptions
		WHERE tenant_id = ? AND enabled = 1 AND events LIKE ?
		ORDER BY created_at`, tenantID, pattern,
	)
	if err != nil {
		return nil, fmt.Errorf("listing enabled webhooks by event: %w", err)
	}
	defer rows.Close()

	var subs []WebhookSubscription
	for rows.Next() {
		var sub WebhookSubscription
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.URL, &sub.SecretHash, &sub.Events, &sub.Enabled, &sub.CreatedBy, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning webhook subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// CreateWebhookDelivery inserts a new webhook delivery record.
func (d *DB) CreateWebhookDelivery(ctx context.Context, delivery *WebhookDelivery) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO webhook_deliveries
			(id, tenant_id, subscription_id, event_type, status, http_status, response_time,
			 attempt_number, next_retry_at, delivered_at, attempted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		delivery.ID, delivery.TenantID, delivery.SubscriptionID, delivery.EventType,
		delivery.Status, delivery.HTTPStatus, delivery.ResponseTime,
		delivery.AttemptNumber, delivery.NextRetryAt, delivery.DeliveredAt, delivery.AttemptedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting webhook delivery: %w", err)
	}
	return nil
}

// UpdateWebhookDeliveryStatus updates the status, http_status, response_time, and
// delivered_at timestamp of a webhook delivery.
func (d *DB) UpdateWebhookDeliveryStatus(ctx context.Context, id, status string, httpStatus *int, responseTime *int) error {
	var deliveredAt *string
	if status == "success" {
		ts := Now()
		deliveredAt = &ts
	}

	result, err := d.conn.ExecContext(ctx,
		`UPDATE webhook_deliveries
		SET status = ?, http_status = ?, response_time = ?, delivered_at = ?
		WHERE id = ?`,
		status, httpStatus, responseTime, deliveredAt, id,
	)
	if err != nil {
		return fmt.Errorf("updating webhook delivery status: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("webhook delivery not found: %s", id)
	}
	return nil
}

// ListWebhookDeliveries returns deliveries for a subscription, ordered by
// attempted_at descending, with limit and offset for pagination.
func (d *DB) ListWebhookDeliveries(ctx context.Context, subscriptionID string, limit, offset int) ([]WebhookDelivery, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, tenant_id, subscription_id, event_type, status, http_status,
			response_time, attempt_number, next_retry_at, delivered_at, attempted_at
		FROM webhook_deliveries WHERE subscription_id = ?
		ORDER BY attempted_at DESC
		LIMIT ? OFFSET ?`, subscriptionID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing webhook deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []WebhookDelivery
	for rows.Next() {
		var del WebhookDelivery
		if err := rows.Scan(
			&del.ID, &del.TenantID, &del.SubscriptionID, &del.EventType,
			&del.Status, &del.HTTPStatus, &del.ResponseTime, &del.AttemptNumber,
			&del.NextRetryAt, &del.DeliveredAt, &del.AttemptedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning webhook delivery: %w", err)
		}
		deliveries = append(deliveries, del)
	}
	return deliveries, rows.Err()
}
