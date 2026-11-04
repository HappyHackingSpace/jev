// Command jev asks TypeSafe's Jev typed questions from the command line and
// serves them as MCP tools.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/happyhackingspace/jev/internal/cli"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Execute(ctx, version, os.Args[1:])
	stop()
	os.Exit(code)
}
