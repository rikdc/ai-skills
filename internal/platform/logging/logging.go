// Package logging builds the process logger for text and JSON output.
package logging

import (
	"fmt"
	"io"
	"log/slog"
)

// Options controls logger construction.
type Options struct {
	Format    string // text|json
	Level     slog.Level
	AddSource bool
}

// New returns a logger writing to w, or an error for an unsupported Format.
func New(w io.Writer, opts Options) (*slog.Logger, error) {
	handlerOpts := &slog.HandlerOptions{Level: opts.Level, AddSource: opts.AddSource}
	switch opts.Format {
	case "text":
		return slog.New(slog.NewTextHandler(w, handlerOpts)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(w, handlerOpts)), nil
	default:
		return nil, fmt.Errorf("unsupported log format %q (want text or json)", opts.Format)
	}
}

// Level maps the CLI verbosity flags to a slog level.
func Level(verbose, quiet, debug bool) slog.Level {
	switch {
	case debug, verbose:
		return slog.LevelDebug
	case quiet:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
