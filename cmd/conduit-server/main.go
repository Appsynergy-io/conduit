package main

import (
	"context"
	"fmt"
	"log/slog"
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
	var tlsResult *shared.DevTLSResult
	if dev {
		tlsResult, err = shared.GenerateDevTLS()
		if err != nil {
			return fmt.Errorf("generating dev TLS: %w", err)
		}
		logger.InfoContext(ctx, "dev TLS certificate generated",
			"fingerprint", tlsResult.Fingerprint,
		)
	}

	var tlsCfg = shared.ProductionTLSConfig()
	if tlsResult != nil {
		tlsCfg = tlsResult.TLSConfig
	}

	// Create and start server
	srv := server.New(cfg, database, jwtMgr, tlsCfg, logger, conduit.FrontendFS)
	if err := srv.Start(ctx); err != nil {
		return fmt.Errorf("starting server: %w", err)
	}

	logger.InfoContext(ctx, "server ready",
		"addr", cfg.Server.HTTPAddr,
	)

	// Wait for shutdown signal
	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.ErrorContext(shutdownCtx, "shutdown error", "error", err)
		return err
	}

	logger.InfoContext(context.Background(), "shutdown complete")
	return nil
}
