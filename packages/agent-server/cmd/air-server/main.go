package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree"
	"github.com/spf13/cobra"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/api"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/config"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-server/internal/stream"
)

var version = "dev"

func newRootCmd(cfg *config.Config) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "air-server",
		Short: "Agent Status central REST & streaming server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), *cfg)
		},
	}

	rootCmd.Version = version
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	flags := rootCmd.Flags()
	flags.StringVarP(&cfg.Addr, "addr", "a", cfg.Addr, "HTTP listen address (e.g. :4502 or 127.0.0.1:4502)")
	flags.StringVar(&cfg.DBPath, "db", cfg.DBPath, "Path to agent-status SQLite database (read-only)")
	flags.StringVar(&cfg.SocketPath, "socket", cfg.SocketPath, "Path to notify unix domain socket")
	flags.StringVar(&cfg.IngesterURL, "ingester-url", cfg.IngesterURL, "URL of air-ingester for mutation proxying")
	flags.StringSliceVar(&cfg.CORSOrigins, "cors-origin", cfg.CORSOrigins, "Allowed CORS origins")
	flags.StringVar(&cfg.AuthToken, "token", cfg.AuthToken, "Bearer authentication token (or AGENT_STATUS_TOKEN env)")
	flags.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level (debug, info, warn, error)")
	flags.StringVar(&cfg.LogDir, "log-dir", cfg.LogDir, "Directory for rotating log files on disk (default: $XDG_STATE_HOME/agent-status/logs)")
	flags.StringVar(&cfg.LogFile, "log-file", cfg.LogFile, "Custom path for rotating log file on disk")

	cobrahelptree.Setup(rootCmd)

	return rootCmd
}

func main() {
	cfg := config.DefaultConfig()
	rootCmd := newRootCmd(&cfg)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var fileCfg *logging.FileConfig
	if cfg.LogFile != "" || cfg.LogDir != "" {
		fc := logging.LoadFileConfigFromEnv("server")
		if cfg.LogFile != "" {
			fc.Filename = cfg.LogFile
		}
		if cfg.LogDir != "" {
			fc.Dir = cfg.LogDir
		}
		fileCfg = &fc
	}
	logger := logging.SetupWithOptions(logging.SetupOptions{
		Service:    "server",
		Level:      cfg.LogLevel,
		FileConfig: fileCfg,
	})
	defer func() {
		_ = logging.Close()
	}()

	logger.InfoContext(ctx, "db_opened", "path", cfg.DBPath)
	sqliteStore, err := store.OpenReadOnly(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer func() {
		_ = sqliteStore.Close()
	}()

	hub := stream.NewHub()
	defer hub.Close()

	subscriber := stream.NewSubscriber(stream.SubscriberConfig{
		SocketPath:    cfg.SocketPath,
		ReconnectWait: 1 * time.Second,
		Logger:        logger,
	}, sqliteStore, hub)

	go subscriber.Run(ctx)

	server := api.NewServer(api.ServerConfig{
		Store:       sqliteStore,
		Hub:         hub,
		IngesterURL: cfg.IngesterURL,
		AuthToken:   cfg.AuthToken,
		CORSOrigins: cfg.CORSOrigins,
		Logger:      logger,
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.InfoContext(ctx, "server_starting", "addr", cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		logger.Info("server_shutting_down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("server_shutdown_error", "err", err)
		}
		return nil
	case err := <-serverErr:
		return fmt.Errorf("http server error: %w", err)
	}
}
