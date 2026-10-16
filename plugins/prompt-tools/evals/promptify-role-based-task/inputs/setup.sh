#!/usr/bin/env bash
# Prepares an empty scratch repository for the prompt deliverables.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

cat > README.md << 'MDEOF'
# Go Security Review Bot

Deliverables for the prompt-generation task go in this directory:
`analysis.md` and `generated-prompt.md`.
MDEOF
git add README.md
git commit -q -m "chore: scratch repo for prompt deliverables"

echo "Scratch repo ready. Create analysis.md and generated-prompt.md here."
