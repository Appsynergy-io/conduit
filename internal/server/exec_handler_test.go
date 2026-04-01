package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/db"
)

// ---------------------------------------------------------------------------
// POST /api/v1/exec — Bulk Exec
// ---------------------------------------------------------------------------

func TestBulkExec_NoAuth(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	body := `{"command":"uptime","targets":{"all":true,"confirmAll":true}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBulkExec_NonAdmin(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("user", uuid.NewString(), "sess", []string{"org_member"}, []string{"remote-access"})

	body := `{"command":"uptime","targets":{"all":true,"confirmAll":true}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestBulkExec_MissingCommand(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"targets":{"all":true,"confirmAll":true}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBulkExec_MissingTargets(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"command":"uptime","targets":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBulkExec_AllWithoutConfirm(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"command":"uptime","targets":{"all":true}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBulkExec_InvalidTimeout(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"command":"uptime","targets":{"all":true,"confirmAll":true},"timeout":100000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBulkExec_NoMatchingAgents(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	// Target agents by ID, but none connected
	body := `{"command":"uptime","targets":{"agentIds":["` + uuid.NewString() + `"]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBulkExec_UnknownField(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"command":"uptime","targets":{"all":true,"confirmAll":true},"extra":"bad"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBulkExec_InvalidAgentID(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"command":"uptime","targets":{"agentIds":["not-a-uuid"]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// GET /api/v1/exec/{jobId} — Get Job
// ---------------------------------------------------------------------------

func TestGetBulkExecJob_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	tc := 2
	job := &db.BulkExecJob{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Command:     "uptime",
		Status:      "completed",
		TargetCount: &tc,
	}
	require.NoError(t, database.CreateBulkExecJob(ctx, job))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exec/"+job.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, job.ID, resp["id"])
	assert.Equal(t, "completed", resp["status"])
}

func TestGetBulkExecJob_NotFound(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	tenantID := uuid.NewString()
	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exec/"+uuid.NewString(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetBulkExecJob_CrossTenantBlocked(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	require.NoError(t, database.CreateTenant(ctx, tenantB, "B"))

	tc := 1
	job := &db.BulkExecJob{
		ID:          uuid.NewString(),
		TenantID:    tenantB,
		Command:     "whoami",
		Status:      "completed",
		TargetCount: &tc,
	}
	require.NoError(t, database.CreateBulkExecJob(ctx, job))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantA, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exec/"+job.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "cross-tenant access must be blocked")
}

// ---------------------------------------------------------------------------
// POST /api/v1/exec/{jobId}/cancel — Cancel Job
// ---------------------------------------------------------------------------

func TestCancelBulkExec_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	tc := 2
	job := &db.BulkExecJob{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Command:     "sleep 60",
		Status:      "running",
		TargetCount: &tc,
	}
	require.NoError(t, database.CreateBulkExecJob(ctx, job))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec/"+job.ID+"/cancel", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	got, _ := database.GetBulkExecJob(ctx, job.ID)
	assert.Equal(t, "cancelled", got.Status)
}

func TestCancelBulkExec_NonAdmin(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	tc := 1
	job := &db.BulkExecJob{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Command:     "uptime",
		Status:      "running",
		TargetCount: &tc,
	}
	require.NoError(t, database.CreateBulkExecJob(ctx, job))

	token, _ := jwtMgr.IssueAccessToken("user", tenantID, "sess", []string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec/"+job.ID+"/cancel", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCancelBulkExec_AlreadyCompleted(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	tc := 1
	job := &db.BulkExecJob{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Command:     "uptime",
		Status:      "completed",
		TargetCount: &tc,
	}
	require.NoError(t, database.CreateBulkExecJob(ctx, job))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/exec/"+job.ID+"/cancel", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
