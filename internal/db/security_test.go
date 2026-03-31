package db_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 1. SQL Injection Prevention
// ---------------------------------------------------------------------------

func TestSQLInjection_UserEmail(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	// Seed a legitimate user so the table is not empty.
	seedUser(t, d, tenantID, "legit@example.com", "org_member")

	malicious := []string{
		"'; DROP TABLE users; --",
		"' OR '1'='1",
		"admin@test.com' UNION SELECT * FROM users --",
	}

	for _, input := range malicious {
		t.Run(input, func(t *testing.T) {
			got, err := d.GetUserByEmail(ctx, input)
			require.Error(t, err, "expected error for malicious input %q", input)
			assert.True(t, errors.Is(err, sql.ErrNoRows),
				"expected sql.ErrNoRows, got: %v", err)
			assert.Nil(t, got)
		})
	}

	// Confirm the table still exists and the legitimate user is intact.
	users, err := d.ListUsers(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "legit@example.com", users[0].Email)
}

func TestSQLInjection_AgentHostname(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	maliciousHostname := `"; DROP TABLE agents; --`

	a := &db.Agent{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		Hostname:     maliciousHostname,
		AgentKeyHash: "hash-" + uuid.NewString(),
		Status:       "offline",
	}
	err := d.CreateAgent(ctx, a)
	require.NoError(t, err)

	// Retrieve it and verify the malicious string is stored literally as data.
	got, err := d.GetAgentByID(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, maliciousHostname, got.Hostname,
		"malicious hostname should be stored literally, not interpreted as SQL")

	// Table must still exist — listing should work.
	agents, err := d.ListAgents(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, agents, 1)
}

func TestSQLInjection_AuditDetails(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	maliciousDetails := "'; DROP TABLE audit_log; --"

	entry := &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		EventType: "test.injection",
		Details:   &maliciousDetails,
		Outcome:   "success",
	}
	err := d.InsertAuditLog(ctx, entry)
	require.NoError(t, err)

	// Retrieve and verify literal storage.
	logs, err := d.ListAuditLogs(ctx, tenantID, 10, 0)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].Details)
	assert.Equal(t, maliciousDetails, *logs[0].Details,
		"malicious details should be stored literally, not interpreted as SQL")

	// Confirm the table still exists.
	count, err := d.CountAuditLogs(ctx, tenantID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// ---------------------------------------------------------------------------
// 2. Tenant Isolation
// ---------------------------------------------------------------------------

func TestTenantIsolation_Users(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	tenantA := seedTenant(t, d)
	tenantB := seedTenant(t, d)

	seedUser(t, d, tenantA, "alice-a@example.com", "org_member")
	seedUser(t, d, tenantA, "bob-a@example.com", "org_admin")
	seedUser(t, d, tenantB, "charlie-b@example.com", "org_member")

	usersA, err := d.ListUsers(ctx, tenantA)
	require.NoError(t, err)
	assert.Len(t, usersA, 2)
	for _, u := range usersA {
		assert.Equal(t, tenantA, u.TenantID, "tenant A list must only contain tenant A users")
	}

	usersB, err := d.ListUsers(ctx, tenantB)
	require.NoError(t, err)
	assert.Len(t, usersB, 1)
	assert.Equal(t, tenantB, usersB[0].TenantID, "tenant B list must only contain tenant B users")
}

func TestTenantIsolation_Agents(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	tenantA := seedTenant(t, d)
	tenantB := seedTenant(t, d)

	for i := 0; i < 3; i++ {
		a := makeAgent(tenantA)
		require.NoError(t, d.CreateAgent(ctx, a))
	}
	bAgent := makeAgent(tenantB)
	require.NoError(t, d.CreateAgent(ctx, bAgent))

	agentsA, err := d.ListAgents(ctx, tenantA)
	require.NoError(t, err)
	assert.Len(t, agentsA, 3)
	for _, a := range agentsA {
		assert.Equal(t, tenantA, a.TenantID)
	}

	agentsB, err := d.ListAgents(ctx, tenantB)
	require.NoError(t, err)
	assert.Len(t, agentsB, 1)
	assert.Equal(t, tenantB, agentsB[0].TenantID)
}

func TestTenantIsolation_AuditLogs(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	tenantA := seedTenant(t, d)
	tenantB := seedTenant(t, d)

	for i := 0; i < 4; i++ {
		entry := &db.AuditEntry{
			ID:        uuid.NewString(),
			TenantID:  tenantA,
			EventType: "auth.login",
			Outcome:   "success",
		}
		require.NoError(t, d.InsertAuditLog(ctx, entry))
	}
	for i := 0; i < 2; i++ {
		entry := &db.AuditEntry{
			ID:        uuid.NewString(),
			TenantID:  tenantB,
			EventType: "shell.start",
			Outcome:   "success",
		}
		require.NoError(t, d.InsertAuditLog(ctx, entry))
	}

	logsA, err := d.ListAuditLogs(ctx, tenantA, 100, 0)
	require.NoError(t, err)
	assert.Len(t, logsA, 4)
	for _, l := range logsA {
		assert.Equal(t, tenantA, l.TenantID)
	}

	logsB, err := d.ListAuditLogs(ctx, tenantB, 100, 0)
	require.NoError(t, err)
	assert.Len(t, logsB, 2)
	for _, l := range logsB {
		assert.Equal(t, tenantB, l.TenantID)
	}
}

func TestTenantIsolation_Groups(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	tenantA := seedTenant(t, d)
	tenantB := seedTenant(t, d)

	groupA := &db.Group{
		ID:       uuid.NewString(),
		TenantID: tenantA,
		Name:     "engineers-a",
	}
	require.NoError(t, d.CreateGroup(ctx, groupA))

	groupA2 := &db.Group{
		ID:       uuid.NewString(),
		TenantID: tenantA,
		Name:     "ops-a",
	}
	require.NoError(t, d.CreateGroup(ctx, groupA2))

	groupB := &db.Group{
		ID:       uuid.NewString(),
		TenantID: tenantB,
		Name:     "engineers-b",
	}
	require.NoError(t, d.CreateGroup(ctx, groupB))

	groupsA, err := d.ListGroups(ctx, tenantA)
	require.NoError(t, err)
	assert.Len(t, groupsA, 2)
	for _, g := range groupsA {
		assert.Equal(t, tenantA, g.TenantID)
	}

	groupsB, err := d.ListGroups(ctx, tenantB)
	require.NoError(t, err)
	assert.Len(t, groupsB, 1)
	assert.Equal(t, tenantB, groupsB[0].TenantID)
}

// ---------------------------------------------------------------------------
// 3. Data Integrity
// ---------------------------------------------------------------------------

func TestUniqueEmailConstraint(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	tenantA := seedTenant(t, d)
	tenantB := seedTenant(t, d)

	seedUser(t, d, tenantA, "dupe@example.com", "org_member")

	// Same email in same tenant must fail.
	dupeUser := &db.User{
		ID:        uuid.NewString(),
		TenantID:  tenantA,
		Email:     "dupe@example.com",
		FirstName: "Dupe",
		LastName:  "User",
		Role:      "org_member",
		Status:    "active",
	}
	err := d.CreateUser(ctx, dupeUser)
	require.Error(t, err, "duplicate email in same tenant must be rejected")

	// Same email in different tenant must also fail (email is UNIQUE globally).
	crossTenantDupe := &db.User{
		ID:        uuid.NewString(),
		TenantID:  tenantB,
		Email:     "dupe@example.com",
		FirstName: "Cross",
		LastName:  "Tenant",
		Role:      "org_member",
		Status:    "active",
	}
	err = d.CreateUser(ctx, crossTenantDupe)
	require.Error(t, err, "duplicate email across tenants must be rejected (UNIQUE constraint)")
}

func TestForeignKeyConstraint(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	bogusUser := &db.User{
		ID:        uuid.NewString(),
		TenantID:  uuid.NewString(), // non-existent tenant
		Email:     "orphan@example.com",
		FirstName: "Orphan",
		LastName:  "User",
		Role:      "org_member",
		Status:    "active",
	}
	err := d.CreateUser(ctx, bogusUser)
	require.Error(t, err, "inserting user with non-existent tenant_id must fail (FK constraint)")
}

func TestRoleAssignment_CheckConstraint(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "check@example.com", "org_member")

	group := &db.Group{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		Name:     "check-group",
	}
	require.NoError(t, d.CreateGroup(ctx, group))

	conn := d.Conn()

	t.Run("both user_id and group_id fails", func(t *testing.T) {
		_, err := conn.ExecContext(ctx, `
			INSERT INTO role_assignments (id, tenant_id, role, user_id, group_id, created_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`,
			uuid.NewString(), tenantID, "org_member", user.ID, group.ID, user.ID,
		)
		require.Error(t, err, "inserting with both user_id AND group_id must violate CHECK constraint")
	})

	t.Run("neither user_id nor group_id fails", func(t *testing.T) {
		_, err := conn.ExecContext(ctx, `
			INSERT INTO role_assignments (id, tenant_id, role, user_id, group_id, created_by, created_at)
			VALUES (?, ?, ?, NULL, NULL, ?, datetime('now'))`,
			uuid.NewString(), tenantID, "org_member", user.ID,
		)
		require.Error(t, err, "inserting with neither user_id nor group_id must violate CHECK constraint")
	})

	t.Run("only user_id succeeds", func(t *testing.T) {
		_, err := conn.ExecContext(ctx, `
			INSERT INTO role_assignments (id, tenant_id, role, user_id, group_id, created_by, created_at)
			VALUES (?, ?, ?, ?, NULL, ?, datetime('now'))`,
			uuid.NewString(), tenantID, "org_admin", user.ID, user.ID,
		)
		require.NoError(t, err, "inserting with only user_id should succeed")
	})

	t.Run("only group_id succeeds", func(t *testing.T) {
		_, err := conn.ExecContext(ctx, `
			INSERT INTO role_assignments (id, tenant_id, role, user_id, group_id, created_by, created_at)
			VALUES (?, ?, ?, NULL, ?, ?, datetime('now'))`,
			uuid.NewString(), tenantID, "org_member", group.ID, user.ID,
		)
		require.NoError(t, err, "inserting with only group_id should succeed")
	})
}

// ---------------------------------------------------------------------------
// 4. Boundary Values
// ---------------------------------------------------------------------------

func TestMaxLengthInputs(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	longEmail := strings.Repeat("a", 9990) + "@test.com" // ~10000 chars
	longFirst := strings.Repeat("b", 10000)
	longLast := strings.Repeat("c", 10000)

	u := &db.User{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Email:     longEmail,
		FirstName: longFirst,
		LastName:  longLast,
		Role:      "org_member",
		Status:    "active",
	}
	err := d.CreateUser(ctx, u)
	require.NoError(t, err, "SQLite should accept very long strings")

	got, err := d.GetUserByID(ctx, u.ID)
	require.NoError(t, err)

	assert.Equal(t, longEmail, got.Email, "long email must round-trip without truncation")
	assert.Equal(t, longFirst, got.FirstName, "long first_name must round-trip without truncation")
	assert.Equal(t, longLast, got.LastName, "long last_name must round-trip without truncation")
}

func TestUnicodeInputs(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	tests := []struct {
		name      string
		firstName string
		lastName  string
	}{
		{
			name:      "emoji",
			firstName: "\U0001F600\U0001F680\U0001F30D",
			lastName:  "\U0001F4BB\U0001F512\U0001F525",
		},
		{
			name:      "CJK characters",
			firstName: "\u5f20\u4f1f",
			lastName:  "\u7530\u4e2d\u592a\u90ce",
		},
		{
			name:      "RTL Arabic",
			firstName: "\u0645\u062d\u0645\u062f",
			lastName:  "\u0639\u0628\u062f\u0627\u0644\u0644\u0647",
		},
		{
			name:      "mixed scripts",
			firstName: "Caf\u00e9 \u00fc\u00f1\u00ef\u00e7\u00f8d\u00e9",
			lastName:  "\u041f\u0440\u0438\u0432\u0435\u0442 \u4e16\u754c",
		},
		{
			name:      "zero-width and combining",
			firstName: "a\u0300e\u0301o\u0302",
			lastName:  "test\u200bword\u200cjoin",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := &db.User{
				ID:        uuid.NewString(),
				TenantID:  tenantID,
				Email:     uuid.NewString() + "@unicode.test",
				FirstName: tc.firstName,
				LastName:  tc.lastName,
				Role:      "org_member",
				Status:    "active",
			}
			err := d.CreateUser(ctx, u)
			require.NoError(t, err)

			got, err := d.GetUserByID(ctx, u.ID)
			require.NoError(t, err)

			assert.Equal(t, tc.firstName, got.FirstName,
				"Unicode first_name must round-trip correctly")
			assert.Equal(t, tc.lastName, got.LastName,
				"Unicode last_name must round-trip correctly")
		})
	}
}

func TestNullableFieldsRoundtrip(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	t.Run("all nullable fields nil", func(t *testing.T) {
		u := &db.User{
			ID:        uuid.NewString(),
			TenantID:  tenantID,
			Email:     "nil-fields@example.com",
			FirstName: "Nil",
			LastName:  "Fields",
			Role:      "org_member",
			Status:    "active",
			// DisplayName, PasswordHash, CreatedBy, LastLoginAt all nil
		}
		err := d.CreateUser(ctx, u)
		require.NoError(t, err)

		got, err := d.GetUserByID(ctx, u.ID)
		require.NoError(t, err)

		assert.Nil(t, got.DisplayName)
		assert.Nil(t, got.PasswordHash)
		assert.Nil(t, got.CreatedBy)
		assert.Nil(t, got.LastLoginAt)
	})

	t.Run("all nullable fields populated", func(t *testing.T) {
		creator := seedUser(t, d, tenantID, "creator@example.com", "org_admin")

		u := &db.User{
			ID:           uuid.NewString(),
			TenantID:     tenantID,
			Email:        "all-fields@example.com",
			FirstName:    "All",
			LastName:     "Fields",
			DisplayName:  ptr("A. Fields"),
			Role:         "org_member",
			Status:       "active",
			PasswordHash: ptr("$argon2id$test-hash"),
			CreatedBy:    &creator.ID,
		}
		err := d.CreateUser(ctx, u)
		require.NoError(t, err)

		// Set last_login_at via the dedicated method.
		err = d.UpdateUserLastLogin(ctx, u.ID)
		require.NoError(t, err)

		got, err := d.GetUserByID(ctx, u.ID)
		require.NoError(t, err)

		require.NotNil(t, got.DisplayName)
		assert.Equal(t, "A. Fields", *got.DisplayName)

		require.NotNil(t, got.PasswordHash)
		assert.Equal(t, "$argon2id$test-hash", *got.PasswordHash)

		require.NotNil(t, got.CreatedBy)
		assert.Equal(t, creator.ID, *got.CreatedBy)

		require.NotNil(t, got.LastLoginAt)
		assert.NotEmpty(t, *got.LastLoginAt)
	})
}
