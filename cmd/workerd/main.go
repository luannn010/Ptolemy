package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/fileops"
	"github.com/luannn010/ptolemy/internal/gitops"
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

	server := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler: httpapi.NewRouter(httpapi.RouterDeps{
			Sessions:  sessionStore,
			Commands:  commandService,
			CommandDB: commandStore,
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	_ = approveServer.Shutdown(ctx)
}
