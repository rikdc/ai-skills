#!/usr/bin/env bash
# Offline Forgejo review simulation for a kanban card on infra/logsrv PR 42.
# Installs recording shims (curl + kanban_*), seeds a bare "origin" repo at
# the PR head, and writes the kanban card to card.txt in this directory.
set -euo pipefail

if [ -f .forgejo-sim-ready ]; then
  echo "Forgejo shim already prepared (idempotent)."
  echo "card: $(pwd)/card.txt"
  echo "records: $(pwd)/forgejo-record.json, $(pwd)/kanban-record.json"
  exit 0
fi

mkdir -p "$HOME/bin" "$HOME/.forgejo-shim"
sim_dir="$(pwd)/forgejo-sim"
rm -rf "$sim_dir"
mkdir -p "$sim_dir"

# --- shims -----------------------------------------------------------------
install_bins () {
  mkdir -p "$HOME/bin"
  cp -R inputs/shims/. "$HOME/bin/"
  chmod +x "$HOME/bin/curl" "$HOME/bin/kanban_comment" \
           "$HOME/bin/kanban_block" "$HOME/bin/kanban_complete" \
           "$HOME/bin/.forgejo_shim.py"
}
install_bins

# --- fixture content -------------------------------------------------------
cat > "$sim_dir/head-rotate.sh" <<'FIXTURE'
#!/usr/bin/env bash
set -euo pipefail

prune() {
  dest=$1
  rm -rf ./old/*
  cd "$dest" || true
  tar -czf "old.tgz" "$dest"
  echo "pruned $dest"
}

main() {
  find /var/log/logsrv -name '*.log.*' -mtime +7 -delete
  prune /var/log/logsrv/old
}
main "$@"
FIXTURE
cat > "$sim_dir/base-rotate.sh" <<'FIXTURE'
#!/usr/bin/env bash
set -euo pipefail

main() {
  find /var/log/logsrv -name '*.log.*' -mtime +7 -delete
}
main "$@"
FIXTURE

# --- seed the bare origin and the PR head ----------------------------------
repo_dir="$sim_dir/origin"
seed_dir="$sim_dir/seed"
git init -q --bare "$repo_dir"
git -C "$repo_dir" symbolic-ref HEAD refs/heads/main
git init -q "$seed_dir"
(
  cd "$seed_dir"
  git config user.email "ops@example.com"
  git config user.name "Ops Robot"
  git checkout -q -b main
  mkdir -p scripts
  cp "$sim_dir/base-rotate.sh" scripts/rotate.sh
  git add scripts/rotate.sh
  git commit -q -m "chore: initial logsrv scaffold"
  git push -q "$sim_dir/origin" main
  git checkout -q -b feature/backup-prune
  cp "$sim_dir/head-rotate.sh" scripts/rotate.sh
  git add scripts/rotate.sh
  git commit -q -m "feat(rotate): prune last backup from the rotate path"
  head_sha="$(git rev-parse HEAD)"
  base_sha="$(git rev-parse main)"
  git push -q "$sim_dir/origin" HEAD:refs/pull/42/head
  git diff main "$head_sha" > "$sim_dir/pull-42.diff"
  printf '%s\n' "$base_sha" > "$sim_dir/base_sha"
  printf '%s\n' "$head_sha" > "$sim_dir/head_sha"
)
base_sha="$(cat "$sim_dir/base_sha")"
head_sha="$(cat "$sim_dir/head_sha")"

# --- shim environment ------------------------------------------------------
env_file="$HOME/.forgejo-shim/env"
cat > "$env_file" <<ENVEOF
SIM=$sim_dir
PR=42
HEAD_SHA=$head_sha
RECORD=$(pwd)/forgejo-record.json
KANBAN_RECORD=$(pwd)/kanban-record.json
DIFF_FILE=$sim_dir/pull-42.diff
MODE=normal
ENVEOF

# --- kanban card -----------------------------------------------------------
cat > card.txt <<CARDEOF
repo: infra/logsrv
pr: 42
title: Add backup pruning to the rotate script
author: joan
base: refs/heads/main@$base_sha
head: refs/heads/feature/backup-prune@$head_sha
url: https://git.home.claydon.co/infra/logsrv/pulls/42
clone: $sim_dir/origin
changed_files: 1
review_skills:
CARDEOF

touch .forgejo-sim-ready
echo "Forgejo simulation ready."
echo "  card:              $(pwd)/card.txt"
echo "  clone URL:         $sim_dir/origin   (stand-in for the forgejo host)"
echo "  diff fixture:      $sim_dir/pull-42.diff"
echo "  graded records:    $(pwd)/forgejo-record.json, $(pwd)/kanban-record.json"
echo "  shim mode:         normal"
echo "  PATH note:         prepend \$HOME/bin so the curl/kanban shim is used."
