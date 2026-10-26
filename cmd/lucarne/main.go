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
	"strings"
	"syscall"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube"
	"github.com/sperano/lucarne/internal/server"
	"github.com/sperano/lucarne/internal/stream"
	"github.com/sperano/lucarne/web"
)

const (
	defaultListen     = ":8080"
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 2 * time.Minute
	shutdownTimeout   = 10 * time.Second
)

// namespaces is a repeatable flag.
type namespaces []string

func (n *namespaces) String() string { return strings.Join(*n, ",") }

func (n *namespaces) Set(v string) error {
	*n = append(*n, v)
	return nil
}

func main() {
	listen := flag.String("listen", defaultListen, "address to serve HTTP on")
	cluster := flag.String("cluster", "", "name of the cluster on run keys (Kubernetes has none)")
	var allowed namespaces
	flag.Var(&allowed, "namespace", "namespace whose runs may be read; repeat for several, none to serve no runs")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runs, err := newRuns(*cluster, allowed, logger)
	if err != nil {
		logger.Error("read runs", "error", err)
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		logger.Error("listen", "address", *listen, "error", err)
		os.Exit(1)
	}
	if err := run(ctx, listener, runs, logger); err != nil {
		logger.Error("lucarne stopped", "error", err)
		os.Exit(1)
	}
}

// newRuns is the hub reading the runs of cluster's allowlisted namespaces
// through the in-cluster or kubeconfig credentials, nil when none is.
func newRuns(cluster string, allowed []string, logger *slog.Logger) (*stream.Hub, error) {
	if len(allowed) == 0 {
		logger.Warn("no namespace allowlisted: the run endpoints answer 501")
		return nil, nil
	}
	cfg, err := kube.RESTConfig()
	if err != nil {
		return nil, err
	}
	clients, err := kube.NewClients(cfg)
	if err != nil {
		return nil, err
	}
	src, err := kube.New(kube.Config{Cluster: cluster, Namespaces: allowed}, clients)
	if err != nil {
		return nil, err
	}
	return stream.New(stream.Config{Source: src, Logger: logger})
}

// run serves on listener until ctx is done, then shuts down gracefully,
// ending the streams of runs first: they would hold the shutdown open.
func run(ctx context.Context, listener net.Listener, runs *stream.Hub, logger *slog.Logger) error {
	// WriteTimeout stays zero: SSE responses are long-lived.
	srv := &http.Server{
		Handler:           server.New(web.Dist(), server.Options{Runs: runs, Logger: logger}),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	if runs != nil {
		srv.RegisterOnShutdown(runs.Close)
		defer runs.Close()
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
