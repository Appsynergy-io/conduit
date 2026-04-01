package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
	"github.com/appsynergy-io/conduit/internal/protocol"
)

// bulkExecRequest is the request body for POST /api/v1/exec.
type bulkExecRequest struct {
	Command        string      `json:"command"`
	Targets        bulkTargets `json:"targets"`
	Timeout        *int        `json:"timeout"`
	MaxConcurrency *int        `json:"maxConcurrency"`
	Shell          *string     `json:"shell"`
}

// bulkTargets specifies which agents to target.
type bulkTargets struct {
	AgentIDs   []string          `json:"agentIds"`
	Labels     map[string]string `json:"labels"`
	GroupIDs   []string          `json:"groupIds"`
	All        bool              `json:"all"`
	ConfirmAll bool              `json:"confirmAll"`
}

// agentResult tracks per-agent execution state.
type agentResult struct {
	AgentID   string  `json:"agentId"`
	Hostname  string  `json:"hostname"`
	Status    string  `json:"status"` // pending, running, completed, failed, timeout
	ExitCode  *int    `json:"exitCode,omitempty"`
	Stdout    string  `json:"stdout,omitempty"`
	Stderr    string  `json:"stderr,omitempty"`
	Truncated bool    `json:"truncated,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
}

// bulkExecJobResponse is the API response shape.
type bulkExecJobResponse struct {
	ID             string         `json:"id"`
	Command        string         `json:"command"`
	Status         string         `json:"status"`
	TargetCount    *int           `json:"targetCount,omitempty"`
	CompletedCount int            `json:"completedCount"`
	FailedCount    int            `json:"failedCount"`
	Results        []agentResult  `json:"results,omitempty"`
	CreatedBy      *string        `json:"createdBy,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	CompletedAt    *string        `json:"completedAt,omitempty"`
}

// handleBulkExec starts a bulk command execution job.
// POST /api/v1/exec
func (s *Server) handleBulkExec(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	var req bulkExecRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", nil)
		return
	}

	// Validate command
	if req.Command == "" {
		apierror.BadRequest(w, r, "Command is required.", nil)
		return
	}
	if len(req.Command) > 65536 {
		apierror.BadRequest(w, r, "Command exceeds maximum length.", nil)
		return
	}

	// Validate timeout
	timeout := 300
	if req.Timeout != nil {
		timeout = *req.Timeout
		if timeout < 1 || timeout > 86400 {
			apierror.BadRequest(w, r, "Timeout must be between 1 and 86400 seconds.", nil)
			return
		}
	}

	// Validate concurrency
	maxConcurrency := 50
	if req.MaxConcurrency != nil {
		maxConcurrency = *req.MaxConcurrency
		if maxConcurrency < 1 || maxConcurrency > 1000 {
			apierror.BadRequest(w, r, "maxConcurrency must be between 1 and 1000.", nil)
			return
		}
	}

	// Validate targets — at least one selector
	if len(req.Targets.AgentIDs) == 0 && len(req.Targets.Labels) == 0 &&
		len(req.Targets.GroupIDs) == 0 && !req.Targets.All {
		apierror.BadRequest(w, r, "At least one target selector is required.", nil)
		return
	}

	if req.Targets.All && !req.Targets.ConfirmAll {
		apierror.BadRequest(w, r, "confirmAll must be true when targeting all agents.", nil)
		return
	}

	// Validate agent IDs
	for _, id := range req.Targets.AgentIDs {
		if _, err := uuid.Parse(id); err != nil {
			apierror.BadRequest(w, r, fmt.Sprintf("Invalid agent ID: %s", id), nil)
			return
		}
	}
	if len(req.Targets.AgentIDs) > 1000 {
		apierror.BadRequest(w, r, "Maximum 1000 agent IDs.", nil)
		return
	}

	// Validate labels
	if len(req.Targets.Labels) > 50 {
		apierror.BadRequest(w, r, "Maximum 50 label selectors.", nil)
		return
	}
	for k, v := range req.Targets.Labels {
		if !labelKeyPattern.MatchString(k) {
			apierror.BadRequest(w, r, fmt.Sprintf("Invalid label key: %q", k), nil)
			return
		}
		if len(v) > 255 {
			apierror.BadRequest(w, r, "Label value exceeds 255 characters.", nil)
			return
		}
	}

	// Resolve target agents
	agents, err := s.resolveExecTargets(r.Context(), claims.TenantID, req.Targets)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if len(agents) == 0 {
		apierror.BadRequest(w, r, "No agents matched the target criteria.", nil)
		return
	}

	// Cap at 1000 results
	if len(agents) > 1000 {
		agents = agents[:1000]
	}

	// Create job record
	targetCount := len(agents)
	jobID := uuid.NewString()
	job := &db.BulkExecJob{
		ID:          jobID,
		TenantID:    claims.TenantID,
		Command:     req.Command,
		Status:      "running",
		TargetCount: &targetCount,
		CreatedBy:   &claims.Subject,
	}

	// Initialize results
	results := make([]agentResult, len(agents))
	for i, a := range agents {
		results[i] = agentResult{
			AgentID:  a.ID,
			Hostname: a.Hostname,
			Status:   "pending",
		}
	}
	resultsJSON, _ := json.Marshal(results)
	resultsStr := string(resultsJSON)
	job.Results = &resultsStr

	if err := s.db.CreateBulkExecJob(r.Context(), job); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Audit
	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "exec.bulk.started",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(fmt.Sprintf(`{"job_id":"%s","command":"%s","target_count":%d}`, jobID, truncate(req.Command, 100), targetCount)),
		Outcome:   "success",
	})

	// Publish event
	s.publishEvent(r.Context(), claims.TenantID, Event{
		Channel: "exec",
		Type:    "exec.bulk.started",
		Data: map[string]interface{}{
			"jobId":       jobID,
			"targetCount": targetCount,
		},
	})

	// Dispatch execution to agents in background
	go s.dispatchExec(jobID, claims.TenantID, agents, req.Command, timeout, maxConcurrency)

	// Return 202 Accepted
	writeJSON(w, http.StatusAccepted, toBulkExecJobResponse(job, results))
}

// handleGetBulkExecJob returns a bulk exec job by ID.
// GET /api/v1/exec/{jobId}
func (s *Server) handleGetBulkExecJob(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	jobID := chi.URLParam(r, "jobId")

	if _, err := uuid.Parse(jobID); err != nil {
		apierror.BadRequest(w, r, "Invalid job ID format.", nil)
		return
	}

	job, err := s.db.GetBulkExecJob(r.Context(), jobID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if job == nil || job.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Job not found.", nil)
		return
	}

	var results []agentResult
	if job.Results != nil {
		json.Unmarshal([]byte(*job.Results), &results)
	}

	writeJSON(w, http.StatusOK, toBulkExecJobResponse(job, results))
}

// handleCancelBulkExec cancels a running bulk exec job.
// POST /api/v1/exec/{jobId}/cancel
func (s *Server) handleCancelBulkExec(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	jobID := chi.URLParam(r, "jobId")

	if _, err := uuid.Parse(jobID); err != nil {
		apierror.BadRequest(w, r, "Invalid job ID format.", nil)
		return
	}

	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	job, err := s.db.GetBulkExecJob(r.Context(), jobID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if job == nil || job.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Job not found.", nil)
		return
	}

	if job.Status != "pending" && job.Status != "running" {
		apierror.BadRequest(w, r, "Job is not in a cancellable state.", nil)
		return
	}

	if err := s.db.CancelBulkExecJob(r.Context(), jobID); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Cancel active exec contexts
	s.execJobsMu.Lock()
	if cancel, ok := s.execJobs[jobID]; ok {
		cancel()
		delete(s.execJobs, jobID)
	}
	s.execJobsMu.Unlock()

	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "exec.bulk.cancelled",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(fmt.Sprintf(`{"job_id":"%s"}`, jobID)),
		Outcome:   "success",
	})

	s.publishEvent(r.Context(), claims.TenantID, Event{
		Channel: "exec",
		Type:    "exec.bulk.cancelled",
		Data:    map[string]string{"jobId": jobID},
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// resolveExecTargets resolves the target selectors to a list of connected agents.
func (s *Server) resolveExecTargets(ctx context.Context, tenantID string, targets bulkTargets) ([]db.Agent, error) {
	agentIDs := make(map[string]struct{})

	// Explicit agent IDs
	for _, id := range targets.AgentIDs {
		agentIDs[id] = struct{}{}
	}

	// By group
	for _, groupID := range targets.GroupIDs {
		members, err := s.db.ListAgentGroupMembers(ctx, groupID)
		if err != nil {
			return nil, fmt.Errorf("resolving group %s: %w", groupID, err)
		}
		for _, id := range members {
			agentIDs[id] = struct{}{}
		}
	}

	// By labels or all — query the agents table
	if len(targets.Labels) > 0 || targets.All {
		var labelFilters []db.LabelFilter
		for k, v := range targets.Labels {
			labelFilters = append(labelFilters, db.LabelFilter{
				Key:    k,
				Values: []string{v},
			})
		}

		agents, err := s.db.ListAgentsPaginated(ctx, db.AgentListParams{
			TenantID: tenantID,
			Limit:    1000,
			Labels:   labelFilters,
		})
		if err != nil {
			return nil, fmt.Errorf("resolving label targets: %w", err)
		}
		for _, a := range agents {
			agentIDs[a.ID] = struct{}{}
		}
	}

	// Filter to only connected agents within the tenant
	var result []db.Agent
	for id := range agentIDs {
		connAgent := s.agentRegistry.Get(id)
		if connAgent == nil || connAgent.TenantID != tenantID {
			continue
		}
		// Fetch agent details from DB
		agent, err := s.db.GetAgentByID(ctx, id)
		if err != nil || agent == nil || agent.TenantID != tenantID {
			continue
		}
		result = append(result, *agent)
	}

	return result, nil
}

// dispatchExec sends EXEC_START to target agents and collects results.
func (s *Server) dispatchExec(jobID, tenantID string, agents []db.Agent, command string, timeout, maxConcurrency int) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout+30)*time.Second)
	defer cancel()

	// Track the cancel function so we can cancel on user request
	s.execJobsMu.Lock()
	s.execJobs[jobID] = cancel
	s.execJobsMu.Unlock()

	defer func() {
		s.execJobsMu.Lock()
		delete(s.execJobs, jobID)
		s.execJobsMu.Unlock()
	}()

	var mu sync.Mutex
	results := make([]agentResult, len(agents))
	for i, a := range agents {
		results[i] = agentResult{
			AgentID:  a.ID,
			Hostname: a.Hostname,
			Status:   "pending",
		}
	}

	completedCount := 0
	failedCount := 0
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, agent := range agents {
		wg.Add(1)
		sem <- struct{}{} // acquire semaphore

		go func(idx int, ag db.Agent) {
			defer wg.Done()
			defer func() { <-sem }() // release semaphore

			// Update status to running
			mu.Lock()
			results[idx].Status = "running"
			mu.Unlock()

			result := s.execOnAgent(ctx, ag.ID, jobID, command, timeout)
			result.AgentID = ag.ID
			result.Hostname = ag.Hostname

			mu.Lock()
			results[idx] = result
			if result.Status == "completed" {
				completedCount++
			} else {
				failedCount++
			}

			// Persist intermediate results
			resultsJSON, _ := json.Marshal(results)
			s.db.UpdateBulkExecJobResults(ctx, jobID, completedCount, failedCount,
				string(resultsJSON), completedCount+failedCount == len(agents))
			mu.Unlock()

			// Publish per-agent event
			s.publishEvent(ctx, tenantID, Event{
				Channel: "exec",
				Type:    "exec.agent.complete",
				Data: map[string]interface{}{
					"jobId":    jobID,
					"agentId":  ag.ID,
					"hostname": ag.Hostname,
					"status":   result.Status,
					"exitCode": result.ExitCode,
				},
			})
		}(i, agent)
	}

	wg.Wait()

	// Final update
	mu.Lock()
	resultsJSON, _ := json.Marshal(results)
	s.db.UpdateBulkExecJobResults(ctx, jobID, completedCount, failedCount,
		string(resultsJSON), true)
	mu.Unlock()

	s.publishEvent(ctx, tenantID, Event{
		Channel: "exec",
		Type:    "exec.bulk.completed",
		Data: map[string]interface{}{
			"jobId":          jobID,
			"completedCount": completedCount,
			"failedCount":    failedCount,
			"targetCount":    len(agents),
		},
	})
}

// execOnAgent sends an EXEC_START frame to a specific agent and waits for EXEC_DATA + EXEC_EXIT.
func (s *Server) execOnAgent(ctx context.Context, agentID, jobID, command string, timeout int) agentResult {
	result := agentResult{Status: "failed"}

	connAgent := s.agentRegistry.Get(agentID)
	if connAgent == nil {
		return result
	}

	streamID := connAgent.Mux.NextStreamID()
	agentCh := connAgent.Mux.OpenStream(streamID)
	defer connAgent.Mux.CloseStream(streamID)

	// Send EXEC_START
	payload := protocol.ExecStartPayload{
		JobID:   jobID,
		Command: command,
		Timeout: timeout,
	}
	frame, err := protocol.NewFrame(protocol.FrameExecStart, streamID, payload)
	if err != nil {
		return result
	}

	startTime := time.Now()
	if err := connAgent.Mux.Send(ctx, frame); err != nil {
		return result
	}

	// Collect output and wait for exit
	execTimeout := time.Duration(timeout) * time.Second
	timer := time.NewTimer(execTimeout + 5*time.Second)
	defer timer.Stop()

	var output []byte
	for {
		select {
		case f, ok := <-agentCh:
			if !ok {
				result.Status = "failed"
				return result
			}
			switch f.Type {
			case protocol.FrameExecData:
				output = append(output, f.Payload...)
				// Cap at 1MB
				if len(output) > 1<<20 {
					output = output[:1<<20]
					result.Truncated = true
				}
			case protocol.FrameExecExit:
				var exitPayload protocol.ExecExitPayload
				if err := protocol.UnmarshalPayload(f.Payload, &exitPayload); err == nil {
					result.ExitCode = &exitPayload.ExitCode
				}
				result.Status = "completed"
				if result.ExitCode != nil && *result.ExitCode != 0 {
					result.Status = "failed"
				}
				result.Stdout = string(output)
				result.Duration = time.Since(startTime).Seconds()
				return result
			}
		case <-timer.C:
			result.Status = "timeout"
			result.Stdout = string(output)
			result.Duration = time.Since(startTime).Seconds()
			return result
		case <-ctx.Done():
			result.Status = "failed"
			result.Stdout = string(output)
			result.Duration = time.Since(startTime).Seconds()
			return result
		}
	}
}

// toBulkExecJobResponse converts a DB job + results to the API response.
func toBulkExecJobResponse(job *db.BulkExecJob, results []agentResult) bulkExecJobResponse {
	return bulkExecJobResponse{
		ID:             job.ID,
		Command:        job.Command,
		Status:         job.Status,
		TargetCount:    job.TargetCount,
		CompletedCount: job.CompletedCount,
		FailedCount:    job.FailedCount,
		Results:        results,
		CreatedBy:      job.CreatedBy,
		CreatedAt:      job.CreatedAt,
		CompletedAt:    job.CompletedAt,
	}
}
