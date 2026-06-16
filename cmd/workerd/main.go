package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/fileops"
	"github.com/luannn010/ptolemy/internal/gitops"
	"github.com/luannn010/ptolemy/internal/health"
	"github.com/luannn010/ptolemy/internal/httpapi"
	"github.com/luannn010/ptolemy/internal/logging"
	"github.com/luannn010/ptolemy/internal/memory"
	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
	"github.com/luannn010/ptolemy/internal/worktree"
	"github.com/rs/zerolog/log"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logging.Setup(cfg.LogLevel)

	baseStore, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open store")
	}
	defer baseStore.Close()

	sessionStore := session.NewStore(baseStore)
	commandStore := command.NewStore(baseStore)
	engine := policy.NewEngine(policy.LoadRuleset(cfg.PolicyPath))
	approvals := policy.NewApprovals()
	// Raw adapters — the only place these are visible to anything outside their guard.
	workspaceRoot := "."
	worktreeRoot := "./.worktrees"
	rawRunner := terminal.NewRunner()
	rawFileOps := fileops.New(workspaceRoot)
	rawGit := gitops.New(workspaceRoot)
	rawWorktrees := worktree.NewManager(workspaceRoot, worktreeRoot)

	// Guards
	guardedRunner := policy.NewGuardedRunner(engine, approvals, rawRunner, baseStore.SQLDB())
	guardedFileOps := policy.NewGuardedFileOps(engine, approvals, rawFileOps, baseStore.SQLDB())
	guardedGit := policy.NewGuardedGit(engine, approvals, rawGit, workspaceRoot, baseStore.SQLDB())
	guardedWorktree := policy.NewGuardedWorktree(engine, approvals, rawWorktrees, workspaceRoot, baseStore.SQLDB())

	commandService := command.NewService(guardedRunner, commandStore)

	// GuardedFileOps/Git/Worktree are constructed here so services can be migrated
	// onto them in follow-up work (plan §11). The discard assignments document the
	// dependency without requiring a parallel service-layer rewrite in this PR.
	_ = guardedFileOps
	_ = guardedGit
	_ = guardedWorktree

	// Optional Postgres pool for the memory DB. pgxpool.New is lazy — it does not
	// dial here, so an unreachable DB surfaces only at /health Ping time, not at
	// startup. nil pool => Postgres reports "disabled".
	var pgPool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		pool, perr := pgxpool.New(context.Background(), cfg.DatabaseURL)
		if perr != nil {
			log.Warn().Err(perr).Msg("postgres pool init failed; /health will report postgres down")
		} else {
			pgPool = pool
		}
	}
	pgCheck := health.NewPgChecker("postgres", nil)
	if pgPool != nil {
		pgCheck = health.NewPgChecker("postgres", pgPool)
	}
	healthAgg := &health.Aggregator{
		Timeout: time.Duration(cfg.HealthTimeoutMS) * time.Millisecond,
		Checkers: []health.Checker{
			health.NewSQLChecker("workerd", baseStore.SQLDB(), true),
			health.NewHTTPChecker("brain", cfg.BrainBaseURL, "/v1/models", true),
			health.NewHTTPChecker("embedder", cfg.EmbeddingBaseURL, "/v1/models", true),
			pgCheck,
			health.NewHTTPChecker("mcp", cfg.MCPBaseURL, "/health", false),
		},
	}

	server := &http.Server{
		Addr: ":" + cfg.HTTPPort,
		Handler: httpapi.NewRouter(httpapi.RouterDeps{
			Sessions:  sessionStore,
			Commands:  commandService,
			CommandDB: commandStore,
			Health:    healthAgg,
		}),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	approveServer := &http.Server{
		Addr:         "127.0.0.1:" + cfg.ApprovePort,
		Handler:      httpapi.NewApproveRouter(approvals),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// RAG HTTP listener (POST /chat): in-process memory, all interfaces so LAN
	// sub-services and WSL can reach it. Gracefully absent when memory is
	// unconfigured. WriteTimeout is generous: an agentic answer is several LLM
	// round-trips and would be killed mid-response by the 15s used above.
	ragDeps, ragCleanup, ragOK := buildRAGDeps(context.Background())

	// Brain lifecycle skill (guarded llama.cpp start/stop/switch + idle-TTL). Off
	// by default. When enabled, its auto-wake adapter is injected into /chat below,
	// and a LOOPBACK-ONLY control plane is exposed (it can stop GPU processes).
	brainDeps, brainCleanup, brainOK := buildBrainDeps(context.Background(), cfg, engine, approvals, baseStore.SQLDB())
	if brainOK && brainDeps.waker != nil {
		ragDeps.Waker = brainDeps.waker
	}

	var ragServer *http.Server
	if ragOK {
		ragServer = &http.Server{
			Addr:         ":" + cfg.RagPort,
			Handler:      httpapi.NewRAGRouter(ragDeps),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 120 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
	}

	var brainControlServer *http.Server
	if brainOK {
		brainControlServer = &http.Server{
			Addr:         "127.0.0.1:" + cfg.BrainControlPort,
			Handler:      httpapi.NewBrainControlRouter(httpapi.BrainDeps{Brain: brainDeps.guarded}),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 120 * time.Second, // a wake can take seconds (model load)
			IdleTimeout:  60 * time.Second,
		}
	}

	log.Info().
		Str("app_env", cfg.AppEnv).
		Str("http_port", cfg.HTTPPort).
		Str("db_path", cfg.DBPath).
		Msg("starting workerd (mvp rebuild baseline)")

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("http server failed")
		}
	}()
	go func() {
		if err := approveServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("approve server failed")
		}
	}()
	if ragServer != nil {
		log.Info().Str("rag_port", cfg.RagPort).Msg("rag http listening (POST /chat)")
		go func() {
			if err := ragServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal().Err(err).Msg("rag server failed")
			}
		}()
	}
	if brainControlServer != nil {
		log.Info().Str("brain_control_port", cfg.BrainControlPort).Msg("brain control listening (loopback)")
		go func() {
			if err := brainControlServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal().Err(err).Msg("brain control server failed")
			}
		}()
	}

	// Sweep has its own lifetime (a Background root, not tied to HTTP shutdown);
	// MaybeStartSweep owns cancellation — sweepCleanup() stops the goroutine and
	// closes the connection.
	sweepCleanup, sweepEnabled, sweepErr := memory.MaybeStartSweep(context.Background())
	switch {
	case sweepErr != nil:
		log.Error().Err(sweepErr).Msg("memory sweep enabled but failed to start; continuing without it")
	case sweepEnabled:
		log.Info().Msg("memory sweep started")
	default:
		log.Info().Msg("memory sweep disabled (set DATABASE_URL and GC_SWEEP_ENABLED=true to enable)")
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	if sweepCleanup != nil {
		sweepCleanup()
	}
	if pgPool != nil {
		pgPool.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	_ = approveServer.Shutdown(ctx)
	if ragServer != nil {
		_ = ragServer.Shutdown(ctx)
	}
	if brainControlServer != nil {
		_ = brainControlServer.Shutdown(ctx)
	}
	// Close the memory conn only after the RAG server has drained, so in-flight
	// /chat requests finish using it.
	if ragCleanup != nil {
		ragCleanup()
	}
	// Stop the idle loop and the brain process after the control + RAG servers
	// have drained (in-flight wakes/answers finish first).
	if brainCleanup != nil {
		brainCleanup()
	}
}
