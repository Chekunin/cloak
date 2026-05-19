// cloakd is the Cloak daemon. It owns the unlocked vault, the local endpoint
// listeners, and the audit log. The CLI binary (cloak) talks to cloakd over
// a Unix domain socket.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Chekunin/cloak/internal/adapters"
	"github.com/Chekunin/cloak/internal/adapters/httpadapter"
	"github.com/Chekunin/cloak/internal/adapters/mysql"
	"github.com/Chekunin/cloak/internal/adapters/postgres"
	"github.com/Chekunin/cloak/internal/adapters/sshadapter"
	"github.com/Chekunin/cloak/internal/audit"
	"github.com/Chekunin/cloak/internal/config"
	"github.com/Chekunin/cloak/internal/endpoints"
	"github.com/Chekunin/cloak/internal/ipc"
	"github.com/Chekunin/cloak/internal/paths"
	"github.com/Chekunin/cloak/internal/store"
	"github.com/Chekunin/cloak/internal/vault"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "cloakd:", err)
		os.Exit(1)
	}
}

func run() error {
	// -foreground is accepted for compatibility with `cloak daemon start`,
	// which always invokes cloakd with this flag. cloakd itself never
	// detaches; backgrounding is the CLI's responsibility.
	var foreground bool
	flag.BoolVar(&foreground, "foreground", true, "run in the foreground (always true; flag retained for CLI compatibility)")
	flag.Parse()
	_ = foreground

	p, err := paths.Default()
	if err != nil {
		return err
	}
	if err := p.EnsureHome(); err != nil {
		return err
	}

	cfg, err := config.Load(p)
	if err != nil {
		return err
	}

	level := parseLogLevel(cfg.Daemon.LogLevel)
	logger := zerolog.New(os.Stderr).Level(level).With().Timestamp().Logger()
	log.Logger = logger

	// Write PID file for `cloak daemon status`.
	if err := writePIDFile(p.PIDFile()); err != nil {
		logger.Warn().Err(err).Msg("could not write pid file")
	}
	defer func() { _ = os.Remove(p.PIDFile()) }()

	auditLog, err := audit.Open(cfg.Audit.LogPath)
	if err != nil {
		return err
	}
	defer auditLog.Close()

	v, err := vault.New(p.VaultMeta(), cfg.IdleTimeout())
	if err != nil {
		return err
	}
	st, err := store.Open(p.VaultDB(), v)
	if err != nil {
		return err
	}
	defer st.Close()

	registry := adapters.NewRegistry()
	registry.Register(httpadapter.New())
	registry.Register(postgres.New())
	registry.Register(mysql.New())
	registry.Register(sshadapter.New(cfg.SSH.HostKeyDir))

	em := endpoints.NewManager(registry, v, st, auditLog, cfg.Endpoints.DefaultPersistentPortStart)
	v.RegisterLockHook(func(reason vault.LockReason) {
		em.CloseAll(string(reason))
	})

	server := ipc.New(cfg.Daemon.SocketPath, st, auditLog, logger)
	ipc.RegisterAll(server, ipc.Deps{
		Vault:     v,
		Store:     st,
		Endpoints: em,
		Audit:     auditLog,
		Adapters:  registry,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logger.Info().
		Str("socket", cfg.Daemon.SocketPath).
		Dur("idle_timeout", cfg.IdleTimeout()).
		Str("vault_state", v.State().String()).
		Msg("cloakd ready")

	select {
	case sig := <-sigCh:
		logger.Info().Str("signal", sig.String()).Msg("shutdown requested")
	case <-ctx.Done():
	}

	// Graceful shutdown.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	done := make(chan struct{})
	go func() {
		_ = server.Stop()
		v.Shutdown()
		close(done)
	}()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		logger.Warn().Msg("shutdown timed out; forcing exit")
	}
	return nil
}

func writePIDFile(path string) error {
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600)
}

func parseLogLevel(s string) zerolog.Level {
	switch s {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
