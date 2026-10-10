#!/usr/bin/env bash
# Initialises a git repo with main and a feature branch containing two
# commits, plus a fake `gh` shim so PR creation can be observed without
# network access.
set -euo pipefail

# Fake gh shim: records the pr create invocation so behaviour is verifiable.
mkdir -p "$HOME/bin"
cat > "$HOME/bin/gh" << 'SHIM'
#!/usr/bin/env bash
record="pr-record.json"
cmd="$1"; shift
case "$cmd" in
  pr)
    sub="$1"; shift
    case "$sub" in
      create)
        echo '{}' > "$record"
        python3 - "$record" "$@" << 'PYEOF'
import json, sys
path = sys.argv[1]
args = sys.argv[2:]
record = {"subcommand": "create", "title": None, "body": None, "draft": False, "base": None}
for i, arg in enumerate(args):
    if arg == "--title":
        record["title"] = args[i + 1]
    elif arg == "--body-file":
        record["body"] = open(args[i + 1]).read()
    elif arg == "--draft":
        record["draft"] = True
    elif arg == "--base":
        record["base"] = args[i + 1]
json.dump(record, open(path, "w"), indent=2)
print("https://github.example.com/metrics/replica-routing/pull/1")
PYEOF
        ;;
      ready|edit|status)
        echo "gh $sub: not supported in this scenario"
        exit 1
        ;;
      *)
        echo "gh pr $sub: not supported in this scenario"
        exit 1
        ;;
    esac
    ;;
  auth)
    # Only `gh auth status` is simulated; report authenticated.
    echo "github.com"
    echo "  ✓ Logged in to github.example.com as metrics-dev"
    exit 0
    ;;
  *)
    echo "gh $cmd: not supported in this scenario"
    exit 1
    ;;
esac
SHIM
chmod +x "$HOME/bin/gh"

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

git checkout -q -b main
mkdir -p src
cat > src/router.py << 'PYEOF'
"""Request router for the metrics service."""

ROUTES = {
    "/health": "health",
    "/metrics": "metrics",
}


def route(path: str):
    """Return the handler name for a request path."""
    return ROUTES.get(path)
PYEOF
git add src/router.py
git commit -q -m "chore: initial service scaffold"

git checkout -q -b feature/replica-routing

# Commit 1: read-replica routing feature
cat > src/router.py << 'PYEOF'
"""Request router for the metrics service."""

ROUTES = {
    "/health": "health",
    "/metrics": "metrics",
    "/replica": "replica",
}

REPLICA_HOSTS = ["replica-0.internal", "replica-1.internal"]


def route(path: str):
    """Return the handler name for a request path."""
    return ROUTES.get(path)


def pick_replica(index: int) -> str:
    """Return a read-replica host for read-only queries."""
    return REPLICA_HOSTS[index % len(REPLICA_HOSTS)]
PYEOF
git add src/router.py
git commit -q -m "feat(router): route read-only queries to replicas"

# Commit 2: docs update
mkdir -p docs
cat > docs/ROUTING.md << 'MDEOF'
# Request routing

Read-only queries are routed to a pool of read replicas. The replica is
chosen round-robin per request by `pick_replica`.
MDEOF
git add docs/ROUTING.md
git commit -q -m "docs(router): document replica routing"

git checkout -q main

echo "Repo ready with branch feature/replica-routing (2 commits ahead)."
echo "gh shim installed at $HOME/bin/gh — prepend ~/bin to PATH."
