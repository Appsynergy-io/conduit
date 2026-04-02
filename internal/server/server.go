package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
	"github.com/appsynergy-io/conduit/internal/shared"
)

// Server holds the server state and dependencies.
type Server struct {
	cfg              *shared.Config
	db               *db.DB
	jwtMgr           *auth.JWTManager
	router           chi.Router
	logger           *slog.Logger
	httpSrv          *http.Server
	tlsConfig        *tls.Config
	eventBus         *EventBus
	agentRegistry    *AgentRegistry
	sessionMgr       *SessionManager
	webhooks         *WebhookDeliverer
	frontendFS       fs.FS
	webAuthn         *webauthn.WebAuthn
	webAuthnSessions *auth.WebAuthnSessionStore
	authLimiter      *middleware.RateLimiter
	setupLimiter     *middleware.RateLimiter
	execJobs         map[string]context.CancelFunc
	execJobsMu       sync.Mutex
	quicTransport     *quic.Transport
	quicEarlyListener *quic.EarlyListener
	http3Srv          *http3.Server
	http3Listener     *alpnDemuxListener
}

// New creates a Server with all dependencies wired.
func New(cfg *shared.Config, database *db.DB, jwtMgr *auth.JWTManager, tlsConfig *tls.Config, logger *slog.Logger, frontendFS fs.FS) *Server {
	eb := NewEventBus(logger)
	s := &Server{
		cfg:              cfg,
		db:               database,
		jwtMgr:           jwtMgr,
		logger:           logger,
		tlsConfig:        tlsConfig,
		eventBus:         eb,
		agentRegistry:    NewAgentRegistry(logger),
		sessionMgr:       NewSessionManager(database, logger, eb),
		webhooks:         NewWebhookDeliverer(database, logger),
		frontendFS:       frontendFS,
		webAuthnSessions: auth.NewWebAuthnSessionStore(),
		// Rate limiters (OWASP A07, API2, API4 — anti-brute-force)
		authLimiter: middleware.NewRateLimiter(middleware.RateLimitConfig{
			Rate:     100,
			Interval: 1 * time.Hour,
			Burst:    20,
		}),
		setupLimiter: middleware.NewRateLimiter(middleware.RateLimitConfig{
			Rate:     20,
			Interval: 1 * time.Hour,
			Burst:    10,
		}),
		execJobs: make(map[string]context.CancelFunc),
	}
	s.initWebAuthn()
	s.router = s.buildRouter()
	return s
}

// initWebAuthn configures the WebAuthn relying party.
// Uses the domain from config; falls back to "localhost" for dev mode.
func (s *Server) initWebAuthn() {
	domain := s.cfg.Server.Domain
	if domain == "" {
		if s.cfg.Server.Mode == "dev" {
			domain = "localhost"
		} else {
			s.logger.Warn("WebAuthn not configured: server.domain not set")
			return
		}
	}

	var origins []string
	if s.cfg.Server.Mode == "dev" {
		// Dev mode uses self-signed certs on port 8443
		origins = []string{"https://localhost" + s.cfg.Server.HTTPAddr}
	} else {
		origins = []string{"https://" + domain}
	}

	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "Conduit",
		RPID:          domain,
		RPOrigins:     origins,
	})
	if err != nil {
		s.logger.Error("failed to initialize WebAuthn", "error", err)
		return
	}

	s.webAuthn = wa
	s.logger.Info("WebAuthn initialized", "rpid", domain, "origins", origins)
}

// Router returns the chi router (for testing).
func (s *Server) Router() chi.Router {
	return s.router
}

// extractPort extracts the port number from an address string like ":443" or "0.0.0.0:8443".
// Returns the port string without colon, or "" if unparseable.
func (s *Server) extractPort(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return port
}

// buildRouter assembles the middleware chain and routes.
// Middleware order: Recover → RequestID → SecurityHeaders → MaxBody → RequireJSON → routes
// Auth is applied per-route group, not globally (some routes are public).
func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()

	// Global middleware (applied to all routes)
	r.Use(middleware.Recover)
	r.Use(middleware.RequestID)
	r.Use(middleware.SecurityHeadersWithAltSvc(s.extractPort(s.cfg.Server.HTTPAddr)))
	r.Use(middleware.MaxBody(1 << 20)) // 1MB default

	// Public routes (no auth)
	r.Get("/health", s.handleHealth)
	r.Get("/install.sh", s.handleInstallScript)

	// Public binary download (outside RequireJSON — serves binary files)
	r.Get("/api/v1/download/agent", s.handleDownloadAgent)
	r.Get("/api/v1/download/agent/platforms", s.handleListAvailableBinaries)

	// WebSocket endpoints (outside RequireJSON — WebSocket upgrade is not JSON)
	r.Get("/api/v1/events/stream", s.handleEventStream)
	r.Get("/api/v1/agents/{agentId}/shell/new", s.handleShellSession)
	r.Get("/api/v1/agents/{agentId}/shell/sessions/{sessionId}/ws", s.handleShellAttach)
	r.Get("/agent/v1/connect", s.handleAgentConnect)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.RequireJSON)

		// Setup wizard (public, no auth — only works before setup is complete)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RateLimit(s.setupLimiter))
			r.Get("/setup/status", s.handleSetupStatus)
			r.Post("/setup/configure", s.handleSetupConfigure)
			r.Post("/setup/passkey", s.handleSetupPasskey)
		})

		// Public auth endpoints (no JWT required)
		// Rate limited: max 100 requests/hour per IP (OWASP A07, API2 — anti-brute-force)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RateLimit(s.authLimiter))
			r.Post("/auth/password/login", s.handlePasswordLogin)
			r.Post("/auth/webauthn/login/begin", s.handleWebAuthnLoginBegin)
			r.Post("/auth/webauthn/login/finish", s.handleWebAuthnLoginFinish)
			r.Post("/auth/recovery/verify", s.handleVerifyRecoveryCode)
		})

		// Agent registration (token-based auth, no JWT)
		r.Post("/agents/register", s.handleAgentRegister)

		// Authenticated endpoints
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(s.jwtMgr, s))
			r.Use(middleware.NoCacheHeaders)

			// Auth session management
			r.Get("/auth/me", s.handleAuthMe)
			r.Post("/auth/logout", s.handleLogout)

			// WebAuthn passkey management (authenticated)
			r.Post("/auth/webauthn/register/begin", s.handleWebAuthnRegisterBegin)
			r.Post("/auth/webauthn/register/finish", s.handleWebAuthnRegisterFinish)
			r.Get("/auth/webauthn/credentials", s.handleListPasskeys)
			r.Delete("/auth/webauthn/credentials/{credentialId}", s.handleDeletePasskey)

			// Recovery codes (authenticated — user generates their own codes)
			r.Post("/auth/recovery/generate", s.handleGenerateRecoveryCodes)

			// CI tokens (NIST IA-5 — machine-to-machine auth)
			r.Get("/auth/ci-tokens", s.handleListCITokens)
			r.Post("/auth/ci-tokens", s.handleCreateCIToken)
			r.Delete("/auth/ci-tokens/{tokenId}", s.handleRevokeCIToken)

			// Users
			r.Get("/users", s.handleListUsers)
			r.Post("/users", s.handleCreateUser)
			r.Get("/users/{userId}", s.handleGetUser)
			r.Patch("/users/{userId}", s.handleUpdateUser)
			r.Post("/users/{userId}/recovery/reset", s.handleAdminRecoveryReset)

			// Groups
			r.Get("/groups", s.handleListGroups)
			r.Post("/groups", s.handleCreateGroup)
			r.Get("/groups/{groupId}", s.handleGetGroup)
			r.Patch("/groups/{groupId}", s.handleUpdateGroup)
			r.Delete("/groups/{groupId}", s.handleDeleteGroup)

			// Sessions
			r.Get("/sessions", s.handleListSessions)
			r.Delete("/sessions/{sessionId}", s.handleRevokeSession)

			// Audit
			r.Get("/audit/events", s.handleListAuditEvents)

			// Webhooks
			r.Get("/webhooks", s.handleListWebhooks)
			r.Post("/webhooks", s.handleCreateWebhook)

			// Service-scoped: remote-access
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireService("remote-access"))

				// Agents
				r.Get("/agents", s.handleListAgents)
				r.Get("/agents/{agentId}", s.handleGetAgent)
				r.Patch("/agents/{agentId}", s.handleUpdateAgent)
				r.Delete("/agents/{agentId}", s.handleDeleteAgent)

				// Labels
				r.Get("/labels", s.handleListLabels)

				// Agent file operations
				r.Get("/agents/{agentId}/files", s.handleListFiles)
				r.Get("/agents/{agentId}/files/download", s.handleDownloadFile)
				r.Post("/agents/{agentId}/files/upload", s.handleUploadFile)
				r.Post("/agents/{agentId}/files/delete", s.handleDeleteFile)
				r.Post("/agents/{agentId}/files/rename", s.handleRenameFile)
				r.Post("/agents/{agentId}/files/mkdir", s.handleMkdir)
				r.Get("/agents/{agentId}/files/preview", s.handlePreviewFile)

				// Join tokens
				r.Get("/agents/tokens", s.handleListJoinTokens)
				r.Post("/agents/tokens", s.handleCreateJoinToken)
				r.Delete("/agents/tokens/{tokenId}", s.handleRevokeJoinToken)

				// Bulk exec
				r.Post("/exec", s.handleBulkExec)
				r.Get("/exec/{jobId}", s.handleGetBulkExecJob)
				r.Post("/exec/{jobId}/cancel", s.handleCancelBulkExec)

				// Shell sessions (persistent/resumable)
				r.Get("/shell/sessions", s.handleListAllShellSessions)
				r.Get("/agents/{agentId}/shell/sessions", s.handleListAgentShellSessions)
				r.Get("/agents/{agentId}/shell/sessions/{sessionId}", s.handleGetShellSession)
				r.Patch("/agents/{agentId}/shell/sessions/{sessionId}", s.handleUpdateShellSession)
				r.Delete("/agents/{agentId}/shell/sessions/{sessionId}", s.handleTerminateShellSession)

				// Shell recordings
				r.Get("/recordings", s.handleListRecordings)
				r.Get("/recordings/{recordingId}", s.handleGetRecording)
			})
		})
	})

	// Frontend catch-all (serves static Next.js export for non-API routes)
	if s.frontendFS != nil {
		r.Handle("/*", newFrontendHandler(s.frontendFS))
	}

	return r
}

// Start begins listening on the configured address with TLS.
func (s *Server) Start(ctx context.Context) error {
	s.httpSrv = &http.Server{
		Addr:              s.cfg.Server.HTTPAddr,
		Handler:           s.router,
		TLSConfig:         s.tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
	}

	ln, err := net.Listen("tcp", s.cfg.Server.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.cfg.Server.HTTPAddr, err)
	}

	if s.tlsConfig != nil {
		// Enable HTTP/2 ALPN negotiation on the TLS config.
		configureHTTP2(s.tlsConfig)
		ln = tls.NewListener(ln, s.tlsConfig)
	}

	s.logger.InfoContext(ctx, "server listening",
		"addr", s.cfg.Server.HTTPAddr,
		"mode", s.cfg.Server.Mode,
	)

	// Start webhook delivery workers
	s.webhooks.Start(ctx)

	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.ErrorContext(ctx, "server error", "error", err)
		}
	}()

	// Start QUIC services (agent QUIC + HTTP/3) on a shared UDP listener.
	// Both protocols share one UDP port, demultiplexed by ALPN.
	if s.cfg.Server.QUICAddr != "" && s.tlsConfig != nil {
		if quicErr := s.startQUICServices(ctx); quicErr != nil {
			s.logger.ErrorContext(ctx, "QUIC services failed — agents will use WebSocket only, browsers HTTP/2 only", "error", quicErr)
		}
	}

	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}
	s.logger.InfoContext(ctx, "shutting down server")
	s.sessionMgr.Stop()
	s.webhooks.Stop()
	s.eventBus.StopThrottleTimers()

	s.stopQUICServices()
	return s.httpSrv.Shutdown(ctx)
}
