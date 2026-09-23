package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
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

			syncNoSummarizeSource(cmd, cfg)

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
				NoSummarize:       cfg.NoSummarize,
				NoSummarizeByFlag: cfg.NoSummarizeByFlag,
				NoSummarizeByEnv:  cfg.NoSummarizeByEnv,
				NoSummarizeSource: cfg.NoSummarizeSource,
				LiteLLMBaseURL:    cfg.LiteLLMBaseURL,
				LiteLLMAPIKey:     cfg.LiteLLMAPIKey,
				LiteLLMModel:      cfg.SummarizerModel,
				GeminiAPIKey:      cfg.GeminiAPIKey,
				GeminiModel:       cfg.SummarizerModel,
				AnthropicAPIKey:   cfg.AnthropicAPIKey,
				AnthropicModel:    cfg.SummarizerModel,
			})
			if err != nil {
				return fmt.Errorf("initializing summarizer engine: %w", err)
			}
			logSummarizerStartup(cfg, sumEngine)

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
			mux.HandleFunc("POST /api/v1/internal/agents/{id}/focus", rcv.HandleFocusAgent)
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
			_, _ = runReconciliationSweep(ctx, rec, "startup")

			// Periodic process reconciliation sweep (every 30 seconds)
			go startPeriodicReconciler(ctx, rec, 30*time.Second)

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

type reconcilerRunner interface {
	Reconcile(ctx context.Context, envID string) (int, error)
}

func runReconciliationSweep(ctx context.Context, rec reconcilerRunner, sweepType string) (int, error) {
	recCount, err := rec.Reconcile(ctx, "")
	if err != nil {
		if ctx.Err() == nil {
			switch sweepType {
			case "startup":
				logging.Warn("process_reconciliation_startup_failed", "err", err)
			case "periodic":
				logging.Warn("process_reconciliation_periodic_failed", "err", err)
			}
		}
		return 0, err
	}
	if recCount > 0 {
		switch sweepType {
		case "startup":
			logging.Info("process_reconciliation_startup_complete", "reconciledCount", recCount)
		case "periodic":
			logging.Info("process_reconciliation_periodic_complete", "reconciledCount", recCount)
		}
	}
	return recCount, nil
}

func startPeriodicReconciler(ctx context.Context, rec reconcilerRunner, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = runReconciliationSweep(ctx, rec, "periodic")
		}
	}
}

func syncNoSummarizeSource(cmd *cobra.Command, cfg *config.Config) {
	if cfg == nil {
		return
	}
	if cmd != nil && cmd.Flags().Changed("no-summarize") {
		if cfg.NoSummarize {
			cfg.NoSummarizeByFlag = true
			cfg.NoSummarizeByEnv = false
			cfg.NoSummarizeSource = "flag"
		}
	} else if cfg.NoSummarize {
		cfg.NoSummarizeByFlag = false
		cfg.NoSummarizeByEnv = true
		cfg.NoSummarizeSource = "env"
	}
}

func logSummarizerStartup(cfg *config.Config, sumEngine *summarizer.Summarizer) {
	isDisabled := false
	isEnv := false
	if sumEngine != nil && (sumEngine.DisabledByFlag() || sumEngine.DisabledByEnv()) {
		isDisabled = true
		isEnv = sumEngine.DisabledByEnv()
	} else if cfg != nil && cfg.NoSummarize {
		isDisabled = true
		isEnv = cfg.NoSummarizeByEnv || cfg.NoSummarizeSource == "env"
	}

	if isDisabled {
		source := "flag"
		if isEnv {
			source = "env"
		}
		logging.Info("summarizer_disabled", "source", source)
		return
	}

	if sumEngine != nil {
		initErrors := sumEngine.ProviderInitErrors()
		if len(initErrors) > 0 {
			provNames := make([]string, 0, len(initErrors))
			for name := range initErrors {
				provNames = append(provNames, name)
			}
			sort.Strings(provNames)
			for _, name := range provNames {
				logging.Error("summarizer_provider_init_failed", "provider", name, "err", initErrors[name])
			}
		}

		if sumEngine.HasProviders() {
			for _, p := range sumEngine.ActiveProviders() {
				if p.Endpoint != "" {
					logging.Info("summarizer_ready", "provider", p.Name, "model", p.Model, "endpoint", p.Endpoint)
				} else {
					logging.Info("summarizer_ready", "provider", p.Name, "model", p.Model)
				}
			}
			return
		}

		if len(initErrors) > 0 {
			return
		}
	}

	logging.Warn("summarizer_unconfigured", "reason", "no_credentials", "checked_env", "LITELLM_BASE_URL,LITELLM_API_KEY,OPENAI_API_KEY,GEMINI_API_KEY,ANTHROPIC_API_KEY")
}

func main() {
	cfg := config.Load()
	rootCmd := newRootCmd(cfg)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
