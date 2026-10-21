// Command evalctl is the Go eval harness.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/rikdc/ai-skills/internal/cli"
	"github.com/rikdc/ai-skills/internal/platform/errs"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	return runWithStderr(args, os.Stderr)
}

func runWithStderr(args []string, stderr io.Writer) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	var signalCode atomic.Int32
	done := make(chan struct{})
	go func() {
		select {
		case sig := <-signals:
			signalCode.Store(int32(errs.SignalCode(sig)))
			cancel()
		case <-done:
		}
	}()

	err := cli.Execute(ctx, args)
	close(done)

	if err != nil {
		fmt.Fprintln(stderr, err)
	}
	if code := signalCode.Load(); code != 0 {
		return int(code)
	}
	return errs.Code(err)
}
