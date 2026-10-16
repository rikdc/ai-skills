#!/usr/bin/env bash
# Seeds a flawed deploy script, a baseline git commit, and a ShellCheck
# shim so the review skill's static-analysis step is observable and
# deterministic without a real shellcheck installation.
set -euo pipefail

rm -f review-report.md

cat > deploy.sh << 'DEPLOYEOF'
#!/bin/bash
# deploy.sh — build a release bundle, place it in the web root, and
# notify the deploy webhook.

RELEASE_DIR=/srv/app/releases

deploy_one() {
  local name=$1
  local dest=$RELEASE_DIR/$name
  mkdir -p $dest
  cd $dest
  rm -rf $dest/*
  cp -R "$name"/artifacts/* $dest/
  touch $dest/ready
  printf 'deployed %s\n' "$dest"
}

rotate_and_notify() {
  local stamp=`date +%s`
  local logfile=/tmp/deploy.$stamp.log
  echo "deploy started: $stamp" >> $logfile
  curl -sS -X POST -H "Authorization: Bearer $WEBCONSOLE_TOKEN" "https://webconsole.internal/hooks/deploy?stamp=$stamp"
}

run_admin_task() {
  local task=$1
  eval "$task"
}

for dir in $(ls "$RELEASE_DIR/pending"); do
  deploy_one "$dir"
done

if [[ $# -gt 0 ]]; then
  run_admin_task "$1"
fi

rotate_and_notify
DEPLOYEOF

cat > .deploy-findings.json << 'FINDINGSEOF'
[{"file":"TARGET","line":10,"column":12,"level":"info","code":2086,"message":"Double quote to prevent globbing and word splitting."},{"file":"TARGET","line":11,"column":3,"level":"warning","code":2164,"message":"Use 'cd ... || exit' or 'cd ... || return' in case cd fails."},{"file":"TARGET","line":11,"column":6,"level":"info","code":2086,"message":"Double quote to prevent globbing and word splitting."},{"file":"TARGET","line":12,"column":10,"level":"warning","code":2115,"message":"Use \"${var:?}\" to ensure this never expands to /* ."},{"file":"TARGET","line":12,"column":10,"level":"info","code":2086,"message":"Double quote to prevent globbing and word splitting."},{"file":"TARGET","line":13,"column":29,"level":"info","code":2086,"message":"Double quote to prevent globbing and word splitting."},{"file":"TARGET","line":14,"column":9,"level":"info","code":2086,"message":"Double quote to prevent globbing and word splitting."},{"file":"TARGET","line":19,"column":9,"level":"warning","code":2155,"message":"Declare and assign separately to avoid masking return values."},{"file":"TARGET","line":19,"column":15,"level":"style","code":2006,"message":"Use $(...) notation instead of legacy backticks `...`."},{"file":"TARGET","line":21,"column":36,"level":"info","code":2086,"message":"Double quote to prevent globbing and word splitting."},{"file":"TARGET","line":30,"column":12,"level":"error","code":2045,"message":"Iterating over ls output is fragile. Use globs."}]
FINDINGSEOF

# Baseline commit so "the review must not auto-fix" is observable.
git init -q
git config user.email "dev@example.com"
git config user.name "Dev"
git add deploy.sh
git commit -q --allow-empty -m "chore: import current deploy script"

# ShellCheck shim: replays the genuine findings (codes, lines, levels,
# messages) that a real ShellCheck 0.11 run reports for this exact file,
# so the offline scenario is deterministic. Only the `fix` blocks are
# dropped; explanation and prioritisation work is left to the reviewer.
mkdir -p bin
cat > bin/shellcheck << 'SHIMEOF'
#!/usr/bin/env bash
# Simulated shellcheck for the deploy.sh review scenario.
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

if [[ -s "$target" && "$(basename "$target")" == "deploy.sh" && -f ".deploy-findings.json" ]]; then
  COMMENT_DATA="$(cat .deploy-findings.json)" python3 - "$target" << 'PYEOF'
import json, os, sys
target = sys.argv[1]
comments = json.loads(os.environ["COMMENT_DATA"])
for c in comments:
    c["file"] = target
np = sys.stdout.write
np(json.dumps({"comments": comments}))
PYEOF
  echo
else
  exit 0
fi
SHIMEOF
chmod +x bin/shellcheck

echo "Scenario ready:"
echo "  deploy.sh          — 38-line bash deploy script under review"
echo "  bin/shellcheck     — ShellCheck shim (prepend $PWD/bin to PATH)"
echo "  .deploy-findings.json — shim data"
echo "  git baseline commit 'chore: import current deploy script' created"
