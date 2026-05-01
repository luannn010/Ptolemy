package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/approval"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/httpapi"
	"github.com/luannn010/ptolemy/internal/logging"
	"github.com/luannn010/ptolemy/internal/logs"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/skills"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"

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

	// Phase 8: run SQLite migrations before any stores use the DB.
	if err := store.RunMigrations(context.Background(), baseStore.SQLDB()); err != nil {
		log.Fatal().Err(err).Msg("failed to run database migrations")
	}

	sessionStore := session.NewStore(baseStore)
	commandStore := command.NewStore(baseStore)
	actionStore := action.NewStore(baseStore.SQLDB())
	logStore := logs.NewStore(baseStore.SQLDB())
	approvalStore := approval.NewStore(baseStore.SQLDB())
	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to resolve working directory")
	}
	skillRegistry, err := skills.NewRegistry(workingDir, cfg.SkillDir)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create skill registry")
	}

	runner := terminal.NewTmuxRunner()

	router := httpapi.NewRouter(
		sessionStore,
		commandStore,
		actionStore,
		logStore,
		approvalStore,
		runner,
		skillRegistry,
	)

	server := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Info().
		Str("app_env", cfg.AppEnv).
		Str("http_port", cfg.HTTPPort).
		Str("state_dir", cfg.StateDir).
		Str("db_path", cfg.DBPath).
		Str("skill_dir", cfg.SkillDir).
		Msg("starting workerd")

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("http server failed")
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	sig := <-stop
	log.Info().
		Str("signal", sig.String()).
		Msg("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
	} else {
		log.Info().Msg("http server stopped gracefully")
	}

	log.Info().Msg("workerd exited")
}
