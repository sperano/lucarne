// Command lucarne-e2e serves Lucarne over the fake cluster of
// internal/e2e/scenario, for the browser tests in web/e2e: the real
// server, stream hub and Kubernetes source, the embedded production UI,
// and under /_e2e/ a control API that drives the fake cluster. It is a
// test tool: the image never holds it.
//
// It prints "listening <base URL>" once it serves, and stops on SIGINT or
// SIGTERM.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sperano/lucarne/internal/e2e/scenario"
	"github.com/sperano/lucarne/internal/server"
	"github.com/sperano/lucarne/internal/stream"
	"github.com/sperano/lucarne/web"
)

const (
	defaultListen = "127.0.0.1:0"
	// pollInterval and rereadDelay are short, so that tests see changes
	// soon after they make them.
	pollInterval      = 100 * time.Millisecond
	rereadDelay       = 100 * time.Millisecond
	heartbeat         = time.Second
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 5 * time.Second
)

func main() {
	listen := flag.String("listen", defaultListen, "address to serve HTTP on")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, *listen, logger); err != nil {
		logger.Error("lucarne-e2e stopped", "error", err)
		os.Exit(1)
	}
}

func serve(ctx context.Context, listen string, logger *slog.Logger) error {
	world, err := scenario.New()
	if err != nil {
		return err
	}
	src, err := world.Source()
	if err != nil {
		return err
	}
	hub, err := stream.New(stream.Config{Source: src, Logger: logger, PollInterval: pollInterval, RereadDelay: rereadDelay})
	if err != nil {
		return err
	}
	defer hub.Close()
	mux := http.NewServeMux()
	gate := newStreamGate()
	newControl(world, gate).register(mux)
	mux.Handle("/", gate.wrap(server.New(web.Dist(), server.Options{Runs: hub, Catalog: src, Logger: logger, Heartbeat: heartbeat})))
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: readHeaderTimeout}
	srv.RegisterOnShutdown(hub.Close)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()
	fmt.Printf("listening http://%s\n", listener.Addr())
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
