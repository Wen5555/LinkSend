package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Wen5555/LinkSend/internal/server"
)

func main() {
	flagSet := flag.NewFlagSet("rendezvous", flag.ExitOnError)
	configPath := flagSet.String("config", "configs/server.dev.toml", "server configuration path")
	flagSet.Parse(os.Args[1:])
	cfg, err := server.LoadConfig(*configPath)
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(2)
	}
	srv, err := server.New(cfg)
	if err != nil {
		slog.Error("create signaling server", "error", err)
		os.Exit(2)
	}
	defer srv.Close()
	httpServer := srv.HTTPServer()
	errCh := make(chan error, 1)
	go func() {
		if cfg.TLSCert != "" || cfg.TLSKey != "" {
			errCh <- httpServer.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
			return
		}
		errCh <- httpServer.ListenAndServe()
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err = <-errCh:
		if !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) && err != nil {
			slog.Error("server stopped", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err = httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown", "error", err)
			os.Exit(1)
		}
	}
}
