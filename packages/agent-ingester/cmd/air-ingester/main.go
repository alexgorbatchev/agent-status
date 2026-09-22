package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/logging"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/classifier"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/config"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/notify"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/pricing"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/receiver"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/reconciler"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
	"github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree"
	"github.com/spf13/cobra"
)

var version = "dev"

func newRootCmd(cfg *config.Config) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "air-ingester",
		Short: "Agent Status Central Telemetry Ingestion Daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			var fileCfg *logging.FileConfig
			if cfg.LogFile != "" || cfg.LogDir != "" {
				fc := logging.LoadFileConfigFromEnv("ingester")
				if cfg.LogFile != "" {
					fc.Filename = cfg.LogFile
				}
				if cfg.LogDir != "" {
					fc.Dir = cfg.LogDir
				}
				fileCfg = &fc
			}
			logging.SetupWithOptions(logging.SetupOptions{
				Service:    "ingester",
				Level:      cfg.LogLevel,
				FileConfig: fileCfg,
			})
			defer func() {
				_ = logging.Close()
			}()

			logging.Info("server_starting", "addr", cfg.Addr, "db", cfg.DBPath, "notifySock", cfg.NotifySock, "noSummarize", cfg.NoSummarize)

			st, err := store.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer func() {
				_ = st.Close()
			}()

			priceEngine, err := pricing.NewPricingEngine()
			if err != nil {
				return fmt.Errorf("initializing pricing engine: %w", err)
			}

			sumEngine, err := summarizer.New(summarizer.Config{
				NoSummarize:     cfg.NoSummarize,
				LiteLLMBaseURL:  cfg.LiteLLMBaseURL,
				LiteLLMAPIKey:   cfg.LiteLLMAPIKey,
				LiteLLMModel:    cfg.SummarizerModel,
				GeminiAPIKey:    cfg.GeminiAPIKey,
				GeminiModel:     cfg.SummarizerModel,
				AnthropicAPIKey: cfg.AnthropicAPIKey,
				AnthropicModel:  cfg.SummarizerModel,
			})
			if err != nil {
				return fmt.Errorf("initializing summarizer engine: %w", err)
			}

			pub, err := notify.NewPublisher(cfg.NotifySock)
			if err != nil {
				return fmt.Errorf("initializing notify publisher: %w", err)
			}
			defer func() {
				_ = pub.Close()
			}()

			cls := classifier.New(st, priceEngine, sumEngine)
			defer func() {
				_ = cls.Close()
			}()
			cls.SetOnUpdate(func(agent *types.IAgentState) {
				if pub != nil && agent != nil {
					pub.Publish(types.IServerEvent{
						Type:  "agent_upsert",
						At:    time.Now().UnixMilli(),
						Agent: agent,
					})
				}
			})
			rcv := receiver.New(st, cls, pub, cfg.BearerToken)

			mux := http.NewServeMux()
			mux.Handle("/ws/ingest", rcv)
			mux.HandleFunc("PATCH /api/v1/internal/agents/{id}/visibility", rcv.HandleSetVisibility)
			mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok\n"))
			})

			srv := &http.Server{
				Addr:              cfg.Addr,
				Handler:           mux,
				ReadHeaderTimeout: 10 * time.Second,
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			// Process reconciliation on startup for local environment
			rec := reconciler.New(st, pub)
			if recCount, rErr := rec.Reconcile(ctx, ""); rErr != nil {
				logging.Warn("process_reconciliation_startup_failed", "err", rErr)
			} else if recCount > 0 {
				logging.Info("process_reconciliation_startup_complete", "reconciledCount", recCount)
			}

			// Periodic process reconciliation sweep (every 30 seconds)
			go func() {
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if _, pErr := rec.Reconcile(ctx, ""); pErr != nil && ctx.Err() == nil {
							logging.Warn("process_reconciliation_periodic_failed", "err", pErr)
						}
					}
				}
			}()

			// Hourly retention pruner
			go func() {
				ticker := time.NewTicker(1 * time.Hour)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						now := time.Now().UnixMilli()
						retentionCutoff := now - (48 * 3600 * 1000)
						exitedCutoff := now - (30 * 24 * 3600 * 1000)
						_ = st.PruneOldEventsAndExitedAgents(retentionCutoff, exitedCutoff)
					}
				}
			}()

			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logging.Error("server_listen_error", "err", err)
				}
			}()

			<-ctx.Done()
			logging.Info("server_shutting_down")

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Shutdown(shutdownCtx)
		},
	}

	rootCmd.Version = version
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	rootCmd.Flags().StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP/WebSocket listen address")
	rootCmd.Flags().StringVar(&cfg.BearerToken, "token", cfg.BearerToken, "Bearer authentication token")
	rootCmd.Flags().StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	rootCmd.Flags().StringVar(&cfg.NotifySock, "notify-sock", cfg.NotifySock, "Unix domain socket path for server event notifications")
	rootCmd.Flags().BoolVar(&cfg.NoSummarize, "no-summarize", cfg.NoSummarize, "Disable LLM action summarization")
	rootCmd.Flags().StringVar(&cfg.LiteLLMBaseURL, "litellm-url", cfg.LiteLLMBaseURL, "OpenAI-compatible / LiteLLM proxy base URL")
	rootCmd.Flags().StringVar(&cfg.LiteLLMAPIKey, "litellm-key", cfg.LiteLLMAPIKey, "OpenAI-compatible / LiteLLM API key")
	rootCmd.Flags().StringVar(&cfg.SummarizerModel, "summarizer-model", cfg.SummarizerModel, "Model to use for action summarization")
	rootCmd.Flags().StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level (debug, info, warn, error)")
	rootCmd.Flags().StringVar(&cfg.LogDir, "log-dir", cfg.LogDir, "Directory for rotating log files on disk (default: $XDG_STATE_HOME/agent-status/logs)")
	rootCmd.Flags().StringVar(&cfg.LogFile, "log-file", cfg.LogFile, "Custom path for rotating log file on disk")

	cobrahelptree.Setup(rootCmd)

	return rootCmd
}

func main() {
	cfg := config.Load()
	rootCmd := newRootCmd(cfg)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
