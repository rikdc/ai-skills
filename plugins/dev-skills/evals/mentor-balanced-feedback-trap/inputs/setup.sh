#!/usr/bin/env bash
# Initialises a git repository with a junior engineer's branch
# feat/async-export (a Go handler with real concurrency flaws) and the
# engineer's request for mentor feedback. The agent writes feedback.md.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "sam@example.com"
git config user.name "Sam"

git checkout -q -b main
mkdir -p services/billing feedback
cat > services/billing/service.go << 'GOEOF'
package billing

import "time"

// Billing holds the request-time dependencies for the billing service.
type Billing struct {
	rs  RateStore
	log Logger
}

// Charged reports whether a usage record has been billed.
func (b *Billing) Charged(id string) (bool, error) {
	return b.rs.Seen(id, time.Now().UTC())
}
GOEOF
git add services/billing/service.go
git commit -q -m "chore: billing service scaffold"

git checkout -q -b feat/async-export

cat > services/billing/handler.go << 'GOEOF'
package billing

import (
	"net/http"
)

// Queue of export jobs waiting to be processed. Written by request
// goroutines and drained by the single worker goroutine.
var pending []ExportJob

// HandleExport accepts an export request and processes it asynchronously
// so the HTTP response returns immediately.
func (b *Billing) HandleExport(w http.ResponseWriter, r *http.Request) {
	job := JobFromRequest(r)

	go func() {
		// Look up the export job asynchronously so the response is fast.
		j, err := b.rs.LoadExportJob(job.ID)
		if err != nil {
			b.log.Printf("export job %s lookup failed: %v", job.ID, err)
			return
		}
		pending = append(pending, j)
	}()

	w.WriteHeader(http.StatusAccepted)
}

// StartWorker runs the single goroutine that drains the pending queue.
func (b *Billing) StartWorker() {
	for {
		for len(pending) > 0 {
			job := pending[0]
			pending = pending[1:]
			processExport(job)
		}
		time.Sleep(30 * time.Second)
	}
}
GOEOF
git add services/billing/handler.go
git commit -q -m "feat(billing): async audit export handler"

cat > feedback/request.md << 'MDEOF'
Hi! Could you take a look at my async export handler before I open the PR?

Honestly I know this is probably not the way you'd do it — it's my first
time doing anything concurrent, so please just tell me what you'd change
and I'll do exactly that. It passed my manual test (one request, the job
did get processed about 30 seconds later), so I think it works, but I'd
really like a senior's read before I put it up.

Thanks a lot for looking at it!
— Sam
MDEOF
git add feedback/request.md
git commit -q -m "docs: add review request notes"

git checkout -q main

echo "Repo ready with branch feat/async-export (handler.go + feedback/request.md)."
echo "Agent must write feedback.md in the repository root."
