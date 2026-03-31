package db_test

import (
	"context"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateWebhookSubscription_Success(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "whcreate@test.com", "org_admin")
	ctx := context.Background()

	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		URL:        "https://example.com/hook",
		SecretHash: "secrethash123",
		Events:     `["agent.connected","auth.login"]`,
		Enabled:    true,
		CreatedBy:  &user.ID,
	}
	err := d.CreateWebhookSubscription(ctx, sub)
	require.NoError(t, err)

	got, err := d.GetWebhookSubscriptionByID(ctx, sub.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, sub.ID, got.ID)
	assert.Equal(t, tenantID, got.TenantID)
	assert.Equal(t, "https://example.com/hook", got.URL)
	assert.Equal(t, "secrethash123", got.SecretHash)
	assert.Equal(t, `["agent.connected","auth.login"]`, got.Events)
	assert.True(t, got.Enabled)
	assert.Equal(t, user.ID, *got.CreatedBy)
	assert.NotEmpty(t, got.CreatedAt)
}

func TestListWebhookSubscriptions(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		sub := &db.WebhookSubscription{
			ID:         uuid.NewString(),
			TenantID:   tenantID,
			URL:        "https://example.com/hook",
			SecretHash: "hash",
			Events:     `["agent.connected"]`,
			Enabled:    true,
		}
		require.NoError(t, d.CreateWebhookSubscription(ctx, sub))
	}

	subs, err := d.ListWebhookSubscriptions(ctx, tenantID)
	require.NoError(t, err)
	assert.Len(t, subs, 2)
}

func TestUpdateWebhookSubscription(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		URL:        "https://old.example.com/hook",
		SecretHash: "hash",
		Events:     `["auth.login"]`,
		Enabled:    true,
	}
	require.NoError(t, d.CreateWebhookSubscription(ctx, sub))

	err := d.UpdateWebhookSubscription(ctx, sub.ID, "https://new.example.com/hook", `["auth.login","agent.connected"]`, false)
	require.NoError(t, err)

	got, err := d.GetWebhookSubscriptionByID(ctx, sub.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "https://new.example.com/hook", got.URL)
	assert.Equal(t, `["auth.login","agent.connected"]`, got.Events)
	assert.False(t, got.Enabled)
}

func TestDeleteWebhookSubscription(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		URL:        "https://example.com/hook",
		SecretHash: "hash",
		Events:     `["agent.connected"]`,
		Enabled:    true,
	}
	require.NoError(t, d.CreateWebhookSubscription(ctx, sub))

	err := d.DeleteWebhookSubscription(ctx, sub.ID)
	require.NoError(t, err)

	got, err := d.GetWebhookSubscriptionByID(ctx, sub.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestListEnabledWebhooksByEvent(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	matching := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		URL:        "https://match.example.com/hook",
		SecretHash: "hash1",
		Events:     `["auth.login","agent.connected"]`,
		Enabled:    true,
	}
	nonMatching := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		URL:        "https://nomatch.example.com/hook",
		SecretHash: "hash2",
		Events:     `["agent.disconnected"]`,
		Enabled:    true,
	}
	require.NoError(t, d.CreateWebhookSubscription(ctx, matching))
	require.NoError(t, d.CreateWebhookSubscription(ctx, nonMatching))

	subs, err := d.ListEnabledWebhooksByEvent(ctx, tenantID, "auth.login")
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, matching.ID, subs[0].ID)
}

func TestCreateWebhookDelivery(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		URL:        "https://example.com/hook",
		SecretHash: "hash",
		Events:     `["auth.login"]`,
		Enabled:    true,
	}
	require.NoError(t, d.CreateWebhookSubscription(ctx, sub))

	del := &db.WebhookDelivery{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		SubscriptionID: sub.ID,
		EventType:      "auth.login",
		Status:         "pending",
		AttemptNumber:  1,
		AttemptedAt:    "2025-06-01T12:00:00Z",
	}
	err := d.CreateWebhookDelivery(ctx, del)
	require.NoError(t, err)

	deliveries, err := d.ListWebhookDeliveries(ctx, sub.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, del.ID, deliveries[0].ID)
	assert.Equal(t, "auth.login", deliveries[0].EventType)
	assert.Equal(t, "pending", deliveries[0].Status)
	assert.Equal(t, 1, deliveries[0].AttemptNumber)
}

func TestUpdateWebhookDeliveryStatus(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		URL:        "https://example.com/hook",
		SecretHash: "hash",
		Events:     `["auth.login"]`,
		Enabled:    true,
	}
	require.NoError(t, d.CreateWebhookSubscription(ctx, sub))

	del := &db.WebhookDelivery{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		SubscriptionID: sub.ID,
		EventType:      "auth.login",
		Status:         "pending",
		AttemptNumber:  1,
		AttemptedAt:    "2025-06-01T12:00:00Z",
	}
	require.NoError(t, d.CreateWebhookDelivery(ctx, del))

	err := d.UpdateWebhookDeliveryStatus(ctx, del.ID, "success", intPtr(200), intPtr(150))
	require.NoError(t, err)

	deliveries, err := d.ListWebhookDeliveries(ctx, sub.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "success", deliveries[0].Status)
	assert.Equal(t, 200, *deliveries[0].HTTPStatus)
	assert.Equal(t, 150, *deliveries[0].ResponseTime)
	assert.NotNil(t, deliveries[0].DeliveredAt)
}

func TestListWebhookDeliveries_Pagination(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		URL:        "https://example.com/hook",
		SecretHash: "hash",
		Events:     `["auth.login"]`,
		Enabled:    true,
	}
	require.NoError(t, d.CreateWebhookSubscription(ctx, sub))

	for i := 0; i < 3; i++ {
		del := &db.WebhookDelivery{
			ID:             uuid.NewString(),
			TenantID:       tenantID,
			SubscriptionID: sub.ID,
			EventType:      "auth.login",
			Status:         "pending",
			AttemptNumber:  1,
			AttemptedAt:    "2025-06-01T12:00:00Z",
		}
		require.NoError(t, d.CreateWebhookDelivery(ctx, del))
	}

	page1, err := d.ListWebhookDeliveries(ctx, sub.ID, 2, 0)
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := d.ListWebhookDeliveries(ctx, sub.ID, 2, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 1)
}
