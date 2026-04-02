package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	conduit "github.com/appsynergy-io/conduit"
	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/server"
	"github.com/appsynergy-io/conduit/internal/shared"
)

func main() {
	root := &cobra.Command{
		Use:   "conduit-server",
		Short: "Conduit CE server — secure remote infrastructure management",
		RunE:  run,
	}

	root.Flags().Bool("dev", false, "Enable dev mode (self-signed TLS, port 8443, password auth)")
	root.Flags().String("config", "", "Path to server.yaml config file")

	_ = viper.BindPFlag("server.mode", root.Flags().Lookup("dev"))
	_ = viper.BindPFlag("config", root.Flags().Lookup("config"))

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, _ []string) error {
	ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	dev, _ := cmd.Flags().GetBool("dev")
	configPath, _ := cmd.Flags().GetString("config")

	level := slog.LevelInfo
	if dev {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	// Load config
	cfg, err := shared.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if dev {
		cfg.ApplyDevDefaults()
	}

	logger.InfoContext(ctx, "starting conduit server", "mode", cfg.Server.Mode)

	// Open database
	database, err := db.New(ctx, cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	// Parse JWT TTLs
	accessTTL, err := time.ParseDuration(cfg.Auth.JWTAccessTTL)
	if err != nil {
		return fmt.Errorf("parsing JWT access TTL: %w", err)
	}
	refreshTTL, err := time.ParseDuration(cfg.Auth.JWTRefreshTTL)
	if err != nil {
		return fmt.Errorf("parsing JWT refresh TTL: %w", err)
	}

	// Create JWT manager (generates Ed25519 keypair)
	jwtMgr, err := auth.NewJWTManager("conduit-server", accessTTL, refreshTTL)
	if err != nil {
		return fmt.Errorf("creating JWT manager: %w", err)
	}

	// TLS config
	var tlsCfg *tls.Config
	var acmeHTTPHandler http.Handler

	if dev {
		tlsResult, err := shared.GenerateDevTLS()
		if err != nil {
			return fmt.Errorf("generating dev TLS: %w", err)
		}
		logger.InfoContext(ctx, "dev TLS certificate generated",
			"fingerprint", tlsResult.Fingerprint,
		)
		tlsCfg = tlsResult.TLSConfig
	} else if cfg.Server.Domain != "" {
		// Production with domain: use ACME for Let's Encrypt certificates.
		acmeMgr, acmeErr := shared.NewACMEManager(cfg.Server.Domain, cfg.Server.CertDir, logger)
		if acmeErr != nil {
			return fmt.Errorf("creating ACME manager: %w", acmeErr)
		}
		tlsCfg = shared.ACMETLSConfig(acmeMgr)
		acmeHTTPHandler = acmeMgr.HTTPHandler(nil)
		logger.InfoContext(ctx, "ACME TLS enabled", "domain", cfg.Server.Domain, "cert_dir", cfg.Server.CertDir)
	} else {
		// Production without domain: base TLS config (manual cert management).
		tlsCfg = shared.ProductionTLSConfig()
	}

	tlsCfg.VerifyConnection = shared.PQCVerifyConnection(logger)

	// Create and start server
	srv := server.New(cfg, database, jwtMgr, tlsCfg, logger, conduit.FrontendFS)

	// Initialize setup (generates token if setup not yet complete)
	token, err := srv.InitSetup(ctx)
	if err != nil {
		return fmt.Errorf("initializing setup: %w", err)
	}

	if err := srv.Start(ctx); err != nil {
		return fmt.Errorf("starting server: %w", err)
	}

	// Start ACME HTTP-01 challenge listener on port 80 (production only).
	// Redirects non-challenge traffic to HTTPS.
	var acmeHTTPSrv *http.Server
	if acmeHTTPHandler != nil {
		acmeHTTPSrv = &http.Server{
			Addr:              ":80",
			Handler:           acmeHTTPHandler,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       30 * time.Second,
		}
		go func() {
			if listenErr := acmeHTTPSrv.ListenAndServe(); listenErr != nil && listenErr != http.ErrServerClosed {
				logger.ErrorContext(ctx, "ACME HTTP listener error", "error", listenErr)
			}
		}()
		logger.InfoContext(ctx, "ACME HTTP-01 challenge listener started", "addr", ":80")
	}

	if token != "" {
		fmt.Fprintf(os.Stderr, "\n  Setup token: %s\n\n", token)
		logger.InfoContext(ctx, "setup required — use the token above to complete setup")
	}

	logger.InfoContext(ctx, "server ready",
		"addr", cfg.Server.HTTPAddr,
	)

	// Wait for shutdown signal
	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if acmeHTTPSrv != nil {
		if err := acmeHTTPSrv.Shutdown(shutdownCtx); err != nil {
			logger.ErrorContext(shutdownCtx, "ACME HTTP shutdown error", "error", err)
		}
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.ErrorContext(shutdownCtx, "shutdown error", "error", err)
		return err
	}

	logger.InfoContext(context.Background(), "shutdown complete")
	return nil
}
