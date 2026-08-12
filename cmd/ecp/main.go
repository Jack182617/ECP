package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"ecp/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	application := cli.CLI{Stdout: os.Stdout, Stderr: os.Stderr}
	os.Exit(application.Run(ctx, os.Args[1:]))
}
