// Command lucarne serves Lucarne: the transcript API and the embedded UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/server"
	"github.com/sperano/lucarne/web"
)

const (
	defaultListen     = ":8080"
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 2 * time.Minute
	shutdownTimeout   = 10 * time.Second
)

func main() {
	listen := flag.String("listen", defaultListen, "address to serve HTTP on")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		logger.Error("listen", "address", *listen, "error", err)
		os.Exit(1)
	}
	if err := run(ctx, listener, logger); err != nil {
		logger.Error("lucarne stopped", "error", err)
		os.Exit(1)
	}
}

// run serves on listener until ctx is done, then shuts down gracefully.
func run(ctx context.Context, listener net.Listener, logger *slog.Logger) error {
	// WriteTimeout stays zero: SSE responses are long-lived.
	srv := &http.Server{
		Handler:           server.New(web.Dist(), logger),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	logger.Info("serving", "address", listener.Addr().String(), "contract", contract.SchemaVersion)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
