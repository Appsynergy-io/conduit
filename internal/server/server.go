package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
	"github.com/appsynergy-io/conduit/internal/shared"
)

// Server holds the server state and dependencies.
type Server struct {
	cfg       *shared.Config
	db        *db.DB
	jwtMgr    *auth.JWTManager
	router    chi.Router
	logger    *slog.Logger
	httpSrv   *http.Server
	tlsConfig *tls.Config
}

// New creates a Server with all dependencies wired.
func New(cfg *shared.Config, database *db.DB, jwtMgr *auth.JWTManager, tlsConfig *tls.Config, logger *slog.Logger) *Server {
	s := &Server{
		cfg:       cfg,
		db:        database,
		jwtMgr:    jwtMgr,
		logger:    logger,
		tlsConfig: tlsConfig,
	}
	s.router = s.buildRouter()
	return s
}

// Router returns the chi router (for testing).
func (s *Server) Router() chi.Router {
	return s.router
}

// buildRouter assembles the middleware chain and routes.
// Middleware order: Recover → RequestID → SecurityHeaders → MaxBody → RequireJSON → routes
// Auth is applied per-route group, not globally (some routes are public).
func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()

	// Global middleware (applied to all routes)
	r.Use(middleware.Recover)
	r.Use(middleware.RequestID)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.MaxBody(1 << 20)) // 1MB default

	// Public routes (no auth)
	r.Get("/health", s.handleHealth)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.RequireJSON)

		// Setup wizard (public, no auth — only works before setup is complete)
		r.Group(func(r chi.Router) {
			r.Get("/setup/status", s.handleSetupStatus)
			r.Post("/setup/configure", s.handleSetupConfigure)
			r.Post("/setup/passkey", s.handleSetupPasskey)
		})

		// Public auth endpoints (no JWT required)
		r.Group(func(r chi.Router) {
			r.Post("/auth/password/login", s.handlePasswordLogin)
			r.Post("/auth/webauthn/login/begin", s.handleNotImplemented)
			r.Post("/auth/webauthn/login/finish", s.handleNotImplemented)
		})

		// Authenticated endpoints
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(s.jwtMgr))
			r.Use(middleware.NoCacheHeaders)

			// Users
			r.Get("/users", s.handleListUsers)
			r.Post("/users", s.handleCreateUser)
			r.Get("/users/{userId}", s.handleGetUser)
			r.Patch("/users/{userId}", s.handleUpdateUser)

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
				r.Delete("/agents/{agentId}", s.handleDeleteAgent)

				// Join tokens
				r.Get("/agents/tokens", s.handleListJoinTokens)
				r.Post("/agents/tokens", s.handleCreateJoinToken)
				r.Delete("/agents/tokens/{tokenId}", s.handleRevokeJoinToken)
			})
		})
	})

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
		BaseContext:        func(_ net.Listener) context.Context { return ctx },
	}

	ln, err := net.Listen("tcp", s.cfg.Server.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.cfg.Server.HTTPAddr, err)
	}

	if s.tlsConfig != nil {
		ln = tls.NewListener(ln, s.tlsConfig)
	}

	s.logger.InfoContext(ctx, "server listening",
		"addr", s.cfg.Server.HTTPAddr,
		"mode", s.cfg.Server.Mode,
	)

	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.ErrorContext(ctx, "server error", "error", err)
		}
	}()

	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}
	s.logger.InfoContext(ctx, "shutting down server")
	return s.httpSrv.Shutdown(ctx)
}
