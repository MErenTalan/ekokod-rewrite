// Command ekokod is the single binary for the ekokod platform.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/MErenTalan/ekokod-rewrite/internal/cli"
)

func main() {
	os.Exit(run())
}

// run holds everything that must complete before the process exits,
// including the deferred stop() below. os.Exit terminates immediately and
// runs no deferred calls, so it must never appear inside a function that
// also defers something — that was gocritic's exitAfterDefer finding
// against the previous shape of this function, where defer stop() sat in
// the same scope as os.Exit(1) and would silently never run on the error
// path. Keeping os.Exit in main, called only after run has returned (and
// therefore after its defers have executed), fixes that while keeping the
// same exit codes.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := cli.Execute(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ekokod: %v\n", err)
		return 1
	}
	return 0
}
