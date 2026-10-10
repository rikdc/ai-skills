#!/usr/bin/env bash
# Prepares an empty scratch repository for the prompt deliverables.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

cat > README.md << 'MDEOF'
# Docs Summariser Prompt

The request was: "make a prompt that makes an AI write good documentation
summaries". Deliverables: `assumptions.md` and `summary-prompt.md`.
MDEOF
git add README.md
git commit -q -m "chore: scratch repo for docs summariser prompt"

echo "Scratch repo ready. Create assumptions.md and summary-prompt.md here."
