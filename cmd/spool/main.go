package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Hopetree/spool/internal/api"
	"github.com/Hopetree/spool/internal/audit"
	"github.com/Hopetree/spool/internal/auth"
	"github.com/Hopetree/spool/internal/config"
	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/host"
	"github.com/Hopetree/spool/internal/lockfile"
	"github.com/Hopetree/spool/internal/sshx"
	"github.com/Hopetree/spool/internal/store"
	"github.com/Hopetree/spool/internal/web"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	setupLogger(cfg.Logging.Level)

	for _, dir := range []string{cfg.DataDir, filepath.Join(cfg.DataDir, "logs"), filepath.Join(cfg.DataDir, "tmp")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	release, err := lockfile.Acquire(filepath.Join(cfg.DataDir, "spool.lock"))
	if err != nil {
		return err
	}
	defer release()

	st, err := store.Open(filepath.Join(cfg.DataDir, "spool.db"))
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(); err != nil {
		return err
	}

	authSvc := auth.NewService(st.DB, time.Duration(cfg.Security.SessionTTLHours)*time.Hour)
	auditSvc := audit.NewService(st.DB)

	masterKey, err := credential.LoadOrCreateMasterKey(filepath.Join(cfg.DataDir, "master.key"))
	if err != nil {
		return err
	}
	cipher, err := credential.NewCipher(masterKey)
	if err != nil {
		return err
	}
	if err := credential.CheckMasterKey(context.Background(), st.DB, cipher); err != nil {
		return fmt.Errorf("master key check failed (was data_dir/master.key replaced or lost?): %w", err)
	}
	credSvc := credential.NewService(st.DB, cipher)
	hostSvc := host.NewService(st.DB, credSvc, sshx.NewProber(), cfg.Security.HostKeyPolicy)

	initialPassword, created, err := authSvc.BootstrapAdmin(context.Background())
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("\n========================================\n Initial admin account: admin\n Initial password: %s\n Change it immediately after login.\n========================================\n\n", initialPassword)
	}

	router := api.NewRouter(api.Deps{
		Cfg:         cfg,
		Store:       st,
		Auth:        authSvc,
		Audit:       auditSvc,
		Credentials: credSvc,
		Hosts:       hostSvc,
		Version:     version,
		StartedAt:   time.Now(),
		WebHandler:  web.Handler(),
	})

	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("spool listening", "addr", cfg.Server.Listen, "version", version)
		var err error
		if cfg.Server.TLS.Cert != "" && cfg.Server.TLS.Key != "" {
			err = srv.ListenAndServeTLS(cfg.Server.TLS.Cert, cfg.Server.TLS.Key)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

func setupLogger(levelName string) {
	var level slog.Level
	switch levelName {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}
