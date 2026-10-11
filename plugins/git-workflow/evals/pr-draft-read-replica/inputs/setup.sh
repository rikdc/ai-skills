#!/usr/bin/env bash
# Initialises a git repo with main and a feature branch containing two
# commits, plus a fake `gh` shim so PR creation can be observed without
# network access.
set -euo pipefail

# Fake gh shim: records the pr create invocation so behaviour is verifiable.
mkdir -p "$HOME/bin"
cat > "$HOME/bin/gh" << 'SHIM'
#!/usr/bin/env bash
# Offline stand-in for `gh`: records what a real `gh pr create` would submit.
record="pr-record.json"
cmd="${1:-}"; shift || true
case "$cmd" in
  pr)
    sub="${1:-}"; shift || true
    case "$sub" in
      create)
        head="$(git symbolic-ref --short HEAD 2>/dev/null || true)"
        prog="$(mktemp "${TMPDIR:-/tmp}/gh-shim.XXXXXX")"
        cat > "$prog" << 'PYEOF'
import json, sys

path, head = sys.argv[1], sys.argv[2]
args = sys.argv[3:]
SHORT = {"-t": "--title", "-b": "--body", "-F": "--body-file",
         "-B": "--base", "-H": "--head", "-d": "--draft"}

tokens = []
for arg in args:
    if arg in SHORT:
        tokens.append(SHORT[arg])
    elif arg.startswith("--") and "=" in arg:
        flag, value = arg.split("=", 1)
        tokens.extend([flag, value])
    else:
        tokens.append(arg)

record = {"subcommand": "create", "title": None, "body": None,
          "draft": False, "base": None, "head": head or None}

i = 0
while i < len(tokens):
    tok = tokens[i]
    value = tokens[i + 1] if i + 1 < len(tokens) else None
    takes_value = value is not None and not value.startswith("--")
    if tok in ("--title", "--body", "--base", "--head"):
        if takes_value:
            record[tok[2:]] = value
            i += 2
            continue
        i += 1
    elif tok == "--body-file":
        if takes_value:
            if value == "-":
                record["body"] = sys.stdin.read()
            else:
                try:
                    record["body"] = open(value).read()
                except OSError:
                    record["body"] = None
            i += 2
            continue
        i += 1
    elif tok == "--draft":
        record["draft"] = True
        i += 1
    else:
        i += 1

json.dump(record, open(path, "w"), indent=2)
print("https://github.example.com/metrics/replica-routing/pull/1")
PYEOF
        python3 "$prog" "$record" "$head" "$@"
        status=$?
        rm -f "$prog"
        exit "$status"
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
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\nsetup_repo.sh\n' >> .git/info/exclude
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
