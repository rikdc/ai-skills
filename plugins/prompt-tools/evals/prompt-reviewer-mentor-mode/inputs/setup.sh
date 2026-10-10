#!/usr/bin/env bash
# Prepares a repository with a junior engineer's rough prompt.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p prompts
cat > prompts/onboarding-helper.md << 'MDEOF'
You are an assistant that helps new people joining the team on their first week.
Actually, whenever you're unsure, act as the team professor who knows
everything about internal tools.

Answer every question the new hire asks and give them a good answer. If a
question relates to our build system, mention the build system. Be correct
and complete. Review answers for correctness.

Remember important things about the user across sessions.
MDEOF
git add prompts/onboarding-helper.md
git commit -q -m "feat: onboarding helper prompt by new engineer"

cat > README.md << 'MDEOF'
# Prompt Library

Team AI prompts live in `prompts/`. The onboarding helper was written by a
new engineer; record a mentor-style review in `mentor-review.md` at the repo
root. Do not edit the prompt file.
MDEOF
git add README.md
git commit -q -m "docs: add prompt library readme"

echo "Repo ready with prompts/onboarding-helper.md (rough first prompt)."
