#!/usr/bin/env bash
# Initialises a git repo with a payments service and a fake `gh` shim so a
# reviewer's comments on PR #42 can be fetched offline. The shim answers the
# review-comment/review/diff API calls from fixtures and records every
# write action (reactions, replies, thread resolutions) in gh-record.json,
# so a dry-run triage can be graded by the absence of recorded actions.
set -euo pipefail

mkdir -p "$HOME/bin"
cat > "$HOME/bin/gh" << 'SHIM'
#!/usr/bin/env bash
# Offline gh shim for triage scenarios.
set -euo pipefail
fixture_dir="$(cat .gh_fixture_dir 2>/dev/null || echo "$PWD/.gh-fixtures")"
record="gh-record.json"
cmd="$1"; shift

append_record() {
  python3 - "$record" "$1" << 'PYEOF'
import json, os, sys
path, entry = sys.argv[1], json.loads(sys.argv[2])
rows = json.load(open(path)) if os.path.exists(path) else []
rows.append(entry)
json.dump(rows, open(path, "w"), indent=2)
PYEOF
}

case "$cmd" in
  pr)
    sub="$1"; shift
    case "$sub" in
      view) cat "$fixture_dir/pr.json" ;;
      diff) cat "$fixture_dir/pr.diff" ;;
      *) echo "gh pr $sub: not supported in this scenario"; exit 1 ;;
    esac
    ;;
  api)
    url="$1"; shift
    # Detect a write (-f/-F fields) vs a read.
    is_write=0
    for a in "$@"; do case "$a" in -f|-F) is_write=1 ;; esac; done
    case "$url" in
      */reviews) cat "$fixture_dir/reviews.json" ;;
      */comments)
        if [ "$is_write" -eq 1 ]; then
          python3 - "$record" "$fixture_dir" "$@" << 'PYEOF'
import json, os, sys
path = sys.argv[1]
args = sys.argv[3:]
entry = {"action": "reply", "in_reply_to": None, "body": None}
for i, a in enumerate(args):
    if a == "-f" or a == "-F":
        key, _, val = args[i + 1].partition("=")
        if key == "body":
            entry["body"] = val
        elif key == "in_reply_to":
            entry["in_reply_to"] = val
rows = json.load(open(path)) if os.path.exists(path) else []
rows.append(entry)
json.dump(rows, open(path, "w"), indent=2)
print(json.dumps({"html_url": "https://github.example.com/fintech/payments/pull/42#discussioncomment-new", "id": 999}))
PYEOF
        else
          cat "$fixture_dir/comments.json"
        fi
        ;;
      graphql)
        python3 - "$record" "$@" << 'PYEOF'
import json, os, sys
path = sys.argv[1]
entry = {"action": "resolve", "query": None}
for i, a in enumerate(sys.argv[2:]):
    if a == "-f" or a == "-F":
        key, _, val = sys.argv[3 + i].partition("=")
        if key == "query":
            entry["query"] = val
rows = json.load(open(path)) if os.path.exists(path) else []
rows.append(entry)
json.dump(rows, open(path, "w"), indent=2)
print(json.dumps({"data": {"minimizeComment": {"minimizedComment": {"isMinimized": True}}}}))
PYEOF
        ;;
      */reactions)
        append_record "{\"action\":\"react\",\"comment_id\":\"$(printf '%s\n' "$url" | grep -oE '[0-9]+/reactions$' | grep -oE '^[0-9]+')\",\"content\":\"+1\"}"
        echo '{"content":"+1"}'
        ;;
      *) echo "gh api $url: not supported in this scenario"; exit 1 ;;
    esac
    ;;
  graphql)
    python3 - "$record" "$@" << 'PYEOF'
import json, os, sys
path = sys.argv[1]
entry = {"action": "resolve", "query": None}
for i, a in enumerate(sys.argv[2:]):
    if a == "-f" or a == "-F":
        key, _, val = sys.argv[3 + i].partition("=")
        if key == "query":
            entry["query"] = val
rows = json.load(open(path)) if os.path.exists(path) else []
rows.append(entry)
json.dump(rows, open(path, "w"), indent=2)
print(json.dumps({"data": {"minimizeComment": {"minimizedComment": {"isMinimized": True}}}}))
PYEOF
    ;;
  auth)
    echo "github.com"
    echo "  ✓ Logged in to github.example.com as payment-dev"
    ;;
  *) echo "gh $cmd: not supported in this scenario"; exit 1 ;;
esac
SHIM
chmod +x "$HOME/bin/gh"

git init -q
git config user.email "payment-dev@example.com"
git config user.name "Payment Dev"

mkdir -p src
cat > CLAUDE.md << 'MDEOF'
# Project Conventions

- All Python functions use `snake_case` (enforced by ruff rule N802).
- SQL statements are always built with parameterised queries — never string
  interpolation of user input.
- Every HTTP handler path validates request payloads before use.
MDEOF
cat > src/handler.py << 'PYEOF'
"""HTTP handlers for the payments service."""

import db


def process_payment(request):
    """Charge a payment from the request payload."""
    name = request.args["name"]
    query = "SELECT * FROM users WHERE name = '%s'" % name
    rows = db.query(query)
    return rows[0]
PYEOF
cat > src/service.py << 'PYEOF'
"""Payment business logic."""

from handler import process_payment


def charge(request):
    return {"status": "paid", "id": process_payment(request)["_id"]}
PYEOF
git add CLAUDE.md src
git commit -q -m "feat: payment processing endpoint WIP"

mkdir -p .gh-fixtures
cat > .gh-fixtures/pr.json << 'JEOF'
{"number": 42, "url": "https://github.example.com/fintech/payments/pull/42", "title": "Add payment processing endpoint", "headRefName": "feature/payments"}
JEOF
cat > .gh-fixtures/reviews.json << 'JEOF'
[{"id": 1, "user": {"login": "alice"}, "state": "CHANGES_REQUESTED", "body": "Please address the security and correctness items below."}]
JEOF
cat > .gh-fixtures/comments.json << 'JEOF'
[
  {"id": 201, "path": "src/handler.py", "line": 9, "body": "You call `db.query` without checking whether it can raise — wrap it or let the error propagate deliberately.", "user": {"login": "alice"}, "in_reply_to_id": null, "html_url": "https://github.example.com/fintech/payments/pull/42#discussion_r201", "node_id": "N201"},
  {"id": 202, "path": "src/handler.py", "line": 7, "body": "Please add a docstring to `process_payment`.", "user": {"login": "bob"}, "in_reply_to_id": null, "html_url": "https://github.example.com/fintech/payments/pull/42#discussion_r202", "node_id": "N202"},
  {"id": 203, "path": "src/handler.py", "line": 12, "body": "Interpolating `name` into the SQL string is an SQL injection vulnerability — use a parameterised query.", "user": {"login": "carol"}, "in_reply_to_id": null, "html_url": "https://github.example.com/fintech/payments/pull/42#discussion_r203", "node_id": "N203"}
]
JEOF
cat > .gh-fixtures/pr.diff << 'MDEOF'
diff --git a/src/handler.py b/src/handler.py
+def process_payment(request):
+    name = request.args["name"]
+    query = "SELECT * FROM users WHERE name = '%s'" % name
+    rows = db.query(query)
MDEOF
echo "$PWD/.gh-fixtures" > .gh_fixture_dir

git add .gh-fixtures .gh_fixture_dir
git commit -q -m "chore: add offline gh fixtures for PR 42"

echo '[]' > gh-record.json

# Seed the record with actions already performed by a previous triage pass:
# comment 201 already received a thumbsup; comment 202 already got a reply;
# coverage for that earlier pass is in triage-report-previous.md.
cat > triage-report-previous.md << 'MDEOF'
# Previous triage pass (partial)

| id | category | decision | action |
|----|----------|----------|--------|
| 201 | error-handling | accept | thumbsup reaction added |
| 202 | nit | reject | reply posted, thread resolved |
MDEOF
python3 - << 'PYEOF'
import json
rows = json.load(open("gh-record.json"))
rows.append({"action": "react", "comment_id": "201", "content": "+1"})
rows.append({"action": "reply", "in_reply_to": "202", "body": "Docstring added in the previous pass -- thanks!"})
rows.append({"action": "resolve", "query": "comment 202"})
json.dump(rows, open("gh-record.json", "w"), indent=2)
PYEOF

echo "Repo ready: payments service + gh shim at $HOME/bin/gh (prepend ~/bin to PATH)."
echo "PR #42 'Add payment processing endpoint' has 3 top-level review comments: 201 (already accepted), 202 (already handled), 203 (new security finding)."
echo "This fixture supports IDEMPOTENT re-triage: comments 201 and 202 were already handled in a previous pass (their prior actions are seeded in gh-record.json), comment 203 is new. The shim records any new action."
