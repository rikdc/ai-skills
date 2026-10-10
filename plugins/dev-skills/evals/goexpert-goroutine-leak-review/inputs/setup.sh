#!/usr/bin/env bash
# Materialises the notify service fixture: a watcher goroutine that leaks via
# an unbuffered channel with no exit path, plus a colleague's proposed fix
# that violates several documented Go rules. Files are written into the
# current working directory; run from a fresh scratch directory.
set -euo pipefail

git init -q -b main
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p internal/notify

cat > go.mod << 'EOF'
module example.com/notify

go 1.21
EOF

cat > internal/notify/worker.go << 'EOF'
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
)

// Event is something worth surfacing to the notification sink.
type Event struct {
	Source string
	Body   string
}

// Watcher polls the upstream webhook feed and forwards events to whoever
// reads from Events(). It has been flapping in production: after StartWatch
// returns, the goroutine below keeps running forever.
type Watcher struct {
	url    string
	client *http.Client
	ch     chan Event
}

func NewWatcher(url string) *Watcher {
	return &Watcher{
		url:    url,
		client: http.DefaultClient,
		ch:     make(chan Event), // unbuffered
	}
}

// Events returns the stream of polled events. The caller decides whether to
// read it.
func (w *Watcher) Events() <-chan Event {
	return w.ch
}

func (w *Watcher) StartWatch(ctx context.Context) error {
	go func() {
		for {
			ev, err := w.poll(ctx)
			if err != nil {
				slog.Error("watcher poll failed", "err", err)
				continue
			}
			w.ch <- ev // blocks forever once StartWatch's caller stops reading
		}
	}()
	return nil
}

func (w *Watcher) poll(ctx context.Context) (Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.url, nil)
	if err != nil {
		return Event{}, fmt.Errorf("build poll request: %w", err)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return Event{}, fmt.Errorf("poll webhook feed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Event{}, fmt.Errorf("unexpected status from feed: %s", resp.Status)
	}
	// Body parsing elided: the shape of the response does not matter here.
	return Event{Source: w.url, Body: "parsed"}, nil
}
EOF

cat > PROPOSED_FIX.md << 'EOF'
# Proposed fix (from Sam, PR comment)

The flapping is easy to fix. In `cmd/serve/main.go` we stop reading from
`Events()` during shutdown, so the send blocks. My patch:

1. In the goroutine, replace `w.ch <- ev` with a `time.Sleep(100 *
   time.Millisecond)` before the send so shutdown usually wins the race and
   the goroutine gets out on the next iteration.
2. When the service shuts down, have the *reader* side call
   `defer close(w.ch)` after draining, so the send panics on a closed channel
   instead of hanging — the panic is contained because the goroutine exits.
3. Spawn a second goroutine that watches poll failures and restarts
   `StartWatch` when the feed has been down for a minute; call
   `wg.Add(1)` *inside* that goroutine right at the top of its body so the
   counter only counts goroutines that actually started.
4. While touching this code: in `poll`, the `fmt.Errorf` result is fine but
   `resp.Body.Close()`'s error should be silenced — change
   `defer resp.Body.Close()` to `defer func() { _ = resp.Body.Close() }()`
   so the linter stops complaining.
EOF

git add -A
git commit -q -m "chore: notify service fixture with flapping watcher"

echo "Fixture ready:"
echo "  internal/notify/worker.go — Watcher with a goroutine that blocks forever on an unbuffered channel"
echo "  PROPOSED_FIX.md           — Sam's suggested patch (sleep-race shutdown, reader-side close, wg.Add inside goroutine, silenced error)"
echo "  git: main branch, 1 commit, working tree clean"
