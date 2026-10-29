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
	"sync/atomic"
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

	// defaultVersion is the version of a binary built without setting
	// version, such as one from a plain go build.
	defaultVersion = "dev"
)

// version is the Lucarne build this binary was made from. The Dockerfile
// sets it from its VERSION build argument with -ldflags "-X main.version=...".
var version = defaultVersion

// options are what run serves and how it stops.
type options struct {
	// runs serves the run endpoints; nil answers them 501.
	runs *stream.Hub
	// catalog serves the run list; nil answers it 501.
	catalog server.RunLister
	// drainDelay is how long /readyz answers 503 after the stop signal,
	// before the listener closes, so that the Service stops routing here.
	drainDelay time.Duration
	logger     *slog.Logger
}

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
	drainDelay := flag.Duration("drain-delay", 0, "how long /readyz answers 503 after SIGTERM before the server stops taking connections")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	opts, err := newReaders(*cluster, allowed, logger)
	if err != nil {
		logger.Error("read runs", "error", err)
		os.Exit(1)
	}
	opts.drainDelay, opts.logger = *drainDelay, logger
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		logger.Error("listen", "address", *listen, "error", err)
		os.Exit(1)
	}
	if err := run(ctx, listener, opts); err != nil {
		logger.Error("lucarne stopped", "error", err)
		os.Exit(1)
	}
}

// newReaders are the hub and the run list reading the runs of cluster's
// allowlisted namespaces through the in-cluster or kubeconfig credentials,
// neither when no namespace is allowlisted.
func newReaders(cluster string, allowed []string, logger *slog.Logger) (options, error) {
	if len(allowed) == 0 {
		logger.Warn("no namespace allowlisted: the run endpoints answer 501")
		return options{}, nil
	}
	cfg, err := kube.RESTConfig()
	if err != nil {
		return options{}, err
	}
	clients, err := kube.NewClients(cfg)
	if err != nil {
		return options{}, err
	}
	src, err := kube.New(kube.Config{Cluster: cluster, Namespaces: allowed}, clients)
	if err != nil {
		return options{}, err
	}
	runs, err := stream.New(stream.Config{Source: src, Logger: logger})
	if err != nil {
		return options{}, err
	}
	return options{runs: runs, catalog: src}, nil
}

// run serves on listener until ctx is done, then drains and shuts down
// gracefully, ending the streams of runs first: they would hold the
// shutdown open.
func run(ctx context.Context, listener net.Listener, opts options) error {
	logger := opts.logger
	var draining atomic.Bool
	// WriteTimeout stays zero: SSE responses are long-lived.
	srv := &http.Server{
		Handler: server.New(web.Dist(), server.Options{
			Runs:    opts.runs,
			Catalog: opts.catalog,
			Logger:  logger,
			Ready:   func() bool { return !draining.Load() },
		}),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	if opts.runs != nil {
		srv.RegisterOnShutdown(opts.runs.Close)
		defer opts.runs.Close()
	}
	logger.Info("serving", "address", listener.Addr().String(), "version", version, "contract", contract.SchemaVersion)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	draining.Store(true)
	if err := drain(opts.drainDelay, served, logger); err != nil {
		return err
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

// drain keeps serving for delay while /readyz answers 503, unless the
// server stops first, whose error it returns.
func drain(delay time.Duration, served <-chan error, logger *slog.Logger) error {
	if delay <= 0 {
		return nil
	}
	logger.Info("draining", "delay", delay.String())
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case err := <-served:
		return err
	case <-timer.C:
		return nil
	}
}
