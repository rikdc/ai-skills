#!/usr/bin/env bash
# Initialises a git repo for the metrics ingestion service with the outbound
# webhooks design brief and backlog tickets in place, so a manager
# orchestration plan can be written against a concrete fixture. No network
# access needed.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"
git checkout -q -b main

mkdir -p docs cmd/server
cp "$here/design-webhooks.md" docs/design-webhooks.md
cp "$here/TICKETS.md" TICKETS.md

cat > cmd/server/main.go << 'EOF'
package main

import (
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	log.Fatal(http.ListenAndServe(":8080", mux))
}
EOF

git add -A
git commit -q -m "docs: add webhooks design brief and backlog tickets"

echo "Repo ready on main with docs/design-webhooks.md, TICKETS.md, cmd/server/main.go."
