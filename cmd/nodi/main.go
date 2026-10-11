package main

import (
	"context"
	"fmt"
	"nodi/internal/cli"
	"os"
	"os/signal"
)

var version = "0.1.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := cli.New(version, os.Stdin, os.Stdout).Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "nodi:", err)
		os.Exit(1)
	}
}
