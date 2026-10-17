// Command pwr-tracker collects Power Hit Radio now-playing observations and
// serves the API and web UI.
//
// Usage:
//
//	pwr-tracker [serve]                 run collector + HTTP server (default)
//	pwr-tracker healthcheck             exit 0 if the local server is live (for containers)
//	pwr-tracker backup <dest.db>        write a consistent snapshot of DATABASE_PATH
//	pwr-tracker demo-seed -db <path>    fill an empty database with synthetic demo history
//	pwr-tracker version                 print build information
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata" // embed the IANA database so Europe/Vilnius works on minimal images

	"github.com/aurimasniekis/go-pwr-tracker/internal/api"
	"github.com/aurimasniekis/go-pwr-tracker/internal/collector"
	"github.com/aurimasniekis/go-pwr-tracker/internal/config"
	"github.com/aurimasniekis/go-pwr-tracker/internal/store"
	"github.com/aurimasniekis/go-pwr-tracker/internal/upstream"
	"github.com/aurimasniekis/go-pwr-tracker/internal/version"
	"github.com/aurimasniekis/go-pwr-tracker/web"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "healthcheck":
		err = healthcheck()
	case "backup":
		err = backup(args)
	case "demo-seed":
		err = demoSeed(args)
	case "version", "-version", "--version":
		v := version.Get()
		fmt.Printf("pwr-tracker %s (commit %s, built %s)\n", v.Version, v.Commit, v.Date)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (expected serve, healthcheck, backup, demo-seed, version)\n", cmd)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func serve() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Value = slog.TimeValue(a.Value.Time().UTC())
			}
			return a
		},
	}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer st.Close()

	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = version.UserAgent()
	}
	var fetcher upstream.Fetcher
	switch cfg.FetchMode {
	case config.FetchModeFlareSolverr:
		fetcher = upstream.NewFlareSolverrFetcher(cfg.FlareSolverrURL, cfg.UpstreamURL, cfg.FlareSolverrTimeout, cfg.MaxResponseBytes)
	default:
		fetcher = upstream.NewDirectFetcher(cfg.UpstreamURL, userAgent, cfg.HTTPTimeout, cfg.MaxResponseBytes)
	}

	col := collector.New(fetcher, st, collector.RealClock, log, collector.Options{
		Interval:             cfg.PollInterval,
		StartupDelay:         cfg.StartupDelay,
		MaxBackoff:           cfg.MaxBackoff,
		ContinuityGap:        cfg.ContinuityGap,
		UpstreamTimezone:     cfg.UpstreamTimezone,
		StoreRawPayloads:     cfg.StoreRawPayloads,
		ObservationRetention: cfg.ObservationRetention,
		RawPayloadRetention:  cfg.RawPayloadRetention,
	})

	srv := api.New(st, api.Options{
		Timezone:         cfg.AnalyticsTimezone,
		PollInterval:     cfg.PollInterval,
		StaleAfter:       cfg.StaleAfter,
		ContinuityGap:    cfg.ContinuityGap,
		FetchMode:        fetcher.Mode(),
		CollectorEnabled: cfg.CollectorEnabled,
		NextAttemptAt:    col.NextAttemptAt,
		UI:               web.FS(),
		Log:              log,
	})
	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}

	v := version.Get()
	log.Info("starting pwr-tracker", "version", v.Version, "commit", v.Commit, "listen", ln.Addr().String(),
		"database", cfg.DatabasePath, "upstream", cfg.UpstreamURL, "fetch_mode", fetcher.Mode(),
		"poll_interval", cfg.PollInterval.String(), "timezone", cfg.AnalyticsTimezone.String(),
		"collector_enabled", cfg.CollectorEnabled, "ui_embedded", web.FS() != nil)

	var wg sync.WaitGroup
	if cfg.CollectorEnabled {
		wg.Go(func() { col.Run(ctx) })
	}
	srvErr := make(chan error, 1)
	go func() { srvErr <- httpSrv.Serve(ln) }()

	select {
	case <-ctx.Done():
		log.Info("shutdown requested")
	case err := <-srvErr:
		if !errors.Is(err, http.ErrServerClosed) {
			stop()
			wg.Wait()
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown", "error", err.Error())
	}
	wg.Wait()
	if err := st.Checkpoint(shutdownCtx); err != nil {
		log.Warn("wal checkpoint on shutdown", "error", err.Error())
	}
	log.Info("stopped")
	return nil
}

// healthcheck probes /healthz on the configured listen port. It exists so
// the distroless image (no shell, no curl) can define a HEALTHCHECK.
func healthcheck() error {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz returned %d", resp.StatusCode)
	}
	return nil
}

func backup(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: pwr-tracker backup <destination.db>")
	}
	path := os.Getenv("DATABASE_PATH")
	if path == "" {
		path = "/data/pwr-tracker.db"
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("database %s: %w", path, err)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, path)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.BackupTo(ctx, args[0]); err != nil {
		return err
	}
	fmt.Printf("backup of %s written to %s\n", path, args[0])
	return nil
}
