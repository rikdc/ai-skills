#!/usr/bin/env bash
# Seeds a snapshot script whose dangerous parts are hidden behind a
# `set -euo pipefail` header, a baseline git commit, and a ShellCheck
# shim so the review skill's static-analysis step is observable and
# deterministic without a real shellcheck installation.
set -euo pipefail

rm -f review-report.md

cat > snapshot.sh << 'SNAPEOF'
#!/bin/bash
# snapshot.sh — write a tarball snapshot of /data/live into /srv/snapshots
# and prune snapshots older than the retention window.

set -euo pipefail
IFS=$'\n\t'

readonly SNAPSHOT_ROOT=/srv/snapshots
readonly DATA_DIR=/data/live

snapshot() {
  local stamp
  local target
  stamp=$(date +%F-%H%M)
  target="snap-$stamp.tgz"
  cd "$SNAPSHOT_ROOT"
  tar czf "$target" "$DATA_DIR"
}

prune_old() {
  local days=$1
  find "$SNAPSHOT_ROOT" -name 'snap-*.tgz' -mtime "+$days" -delete
}

list_snapshots() {
  for f in $(ls "$SNAPSHOT_ROOT"); do
    case "$f" in
      snap-*.tgz) ls -lh "$SNAPSHOT_ROOT/$f" ;;
    esac
  done
}

main() {
  snapshot && prune_old "$1"
  list_snapshots
  echo "snapshot complete"
}

main "$@"
SNAPEOF

cat > .snapshot-findings.json << 'FINDINGSEOF'
[{"file":"TARGET","line":26,"column":12,"level":"error","code":2045,"message":"Iterating over ls output is fragile. Use globs."}]
FINDINGSEOF

# Baseline commit so "the review must not auto-fix" is observable.
git init -q
git config user.email "dev@example.com"
git config user.name "Dev"
git add snapshot.sh
git commit -q --allow-empty -m "chore: import snapshot script from nightly cron"

# ShellCheck shim: replays the genuine findings (codes, lines, levels,
# messages) that a real ShellCheck 0.11 run reports for this exact file.
# Note what it does NOT catch: the unchecked `cd` inside a function
# that is called as part of an `&&` chain, and the failure-swallowing
# guard chain itself. Only the `fix` blocks are dropped.
mkdir -p bin
cat > bin/shellcheck << 'SHIMEOF'
#!/usr/bin/env bash
# Simulated shellcheck for the snapshot.sh strict-mode trap scenario.
set -euo pipefail

target=""
fmt="json1"
for arg in "$@"; do
  case "$arg" in
    --format=*) fmt="${arg#--format=}" ;;
    --*) ;;
    *) target="$arg" ;;
  esac
done

if [[ "$fmt" != "json1" ]]; then
  echo "shellcheck shim: only --format=json1 is supported in this scenario" >&2
  exit 0
fi

if [[ -s "$target" && "$(basename "$target")" == "snapshot.sh" && -f ".snapshot-findings.json" ]]; then
  COMMENT_DATA="$(cat .snapshot-findings.json)" python3 - "$target" << 'PYEOF'
import json, os, sys
target = sys.argv[1]
comments = json.loads(os.environ["COMMENT_DATA"])
for c in comments:
    c["file"] = target
sys.stdout.write(json.dumps({"comments": comments}))
PYEOF
  echo
else
  exit 0
fi
SHIMEOF
chmod +x bin/shellcheck

echo "Scenario ready:"
echo "  snapshot.sh           — 39-line bash snapshot script under review"
echo "  bin/shellcheck        — ShellCheck shim (prepend $PWD/bin to PATH)"
echo "  .snapshot-findings.json — shim data"
echo "  git baseline commit 'chore: import snapshot script from nightly cron' created"
