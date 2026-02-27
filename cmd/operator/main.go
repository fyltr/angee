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

	"github.com/fyltr/angee-go/internal/operator"
	"github.com/fyltr/angee-go/internal/root"
)

func main() {
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
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	bind := srv.Cfg.BindAddress
	if bind == "" {
		bind = "127.0.0.1"
	}
	addr := bind + ":" + port
	if err := srv.Start(ctx, addr); err != nil {
		fmt.Fprintf(os.Stderr, "operator error: %s\n", err)
		os.Exit(1)
	}
}
