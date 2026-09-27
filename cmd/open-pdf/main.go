// Command open-pdf runs the open-pdf HTTP service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/linuxsuren/open-pdf/internal/buildinfo"
	"github.com/linuxsuren/open-pdf/internal/config"
	"github.com/linuxsuren/open-pdf/internal/httpapi"
)

func main() {
	var (
		showVersion bool
		selfCheck   bool
		addrFlag    string
	)
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&selfCheck, "self-check", false, "probe GET /healthz on the configured address and exit (for container healthchecks)")
	flag.StringVar(&addrFlag, "addr", "", "listen address (overrides OPEN_PDF_ADDR)")
	flag.Parse()

	if showVersion {
		fmt.Printf("open-pdf %s (commit %s, built %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if addrFlag != "" {
		cfg.Addr = addrFlag
	}

	if selfCheck {
		if err := selfCheckHealth(cfg.Addr); err != nil {
			fmt.Fprintf(os.Stderr, "self-check: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("self-check: ok")
		return
	}

	log := newLogger(cfg)
	slog.SetDefault(log)

	srv, err := httpapi.New(cfg, log)
	if err != nil {
		log.Error("init server", "err", err)
		os.Exit(1)
	}

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("open-pdf listening", "addr", cfg.Addr, "version", buildinfo.Version)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		log.Error("server error", "err", err)
		os.Exit(1)
	case <-sigCtx.Done():
	}

	log.Info("shutting down gracefully", "timeout", cfg.ShutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	log.Info("shutdown complete")
}

func newLogger(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if cfg.LogJSON {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// selfCheckHealth performs a local health probe, usable from minimal
// container images without shell utilities.
func selfCheckHealth(addr string) error {
	host := addr
	if len(host) > 0 && host[0] == ':' {
		host = "127.0.0.1" + host
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + host + "/healthz")
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
