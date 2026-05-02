// angee-operator is the Angee runtime operator daemon.
// It owns ANGEE_ROOT and manages the runtime backend (Docker Compose or Kubernetes).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fyltr/angee/internal/operator"
	"github.com/fyltr/angee/internal/root"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "operator: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	angeeRoot := os.Getenv("ANGEE_ROOT")
	if angeeRoot == "" {
		angeeRoot = root.DefaultAngeeRoot()
	}

	port := os.Getenv("ANGEE_OPERATOR_PORT")
	if port == "" {
		port = "9000"
	}

	srv, err := operator.New(angeeRoot, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	bind := os.Getenv("ANGEE_BIND_ADDRESS")
	if bind == "" {
		bind = srv.Platform.Cfg.BindAddress
	}
	if bind == "" {
		// Loopback default — see internal/config/operator.go DefaultOperatorConfig.
		// This branch only fires when both env and config are unset.
		bind = "127.0.0.1"
	}
	addr := bind + ":" + port
	return srv.Start(ctx, addr)
}
