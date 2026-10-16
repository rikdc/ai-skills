#!/usr/bin/env bash
# Initialises a git repo for the metrics ingestion service with the API keys
# design brief in place, so a manager execution plan can be written against
# a concrete fixture. No network access needed.
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
cp "$here/design-api-keys.md" docs/design-api-keys.md

cat > cmd/server/main.go << 'EOF'
package main

import (
	"log"
	"net/http"
)

// The ingest endpoint is currently unauthenticated and unthrottled.
// See docs/design-api-keys.md for the planned fix.
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	log.Fatal(http.ListenAndServe(":8080", mux))
}
EOF

git add -A
git commit -q -m "docs: add design brief for API key authentication and rate limiting"

echo "Repo ready on main with docs/design-api-keys.md and cmd/server/main.go."
