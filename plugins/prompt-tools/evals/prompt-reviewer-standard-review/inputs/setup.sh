#!/usr/bin/env bash
# Prepares a repository with a flawed AI prompt awaiting review.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p prompts
cat > prompts/sql-generator.md << 'MDEOF'
You are a helpful assistant.

Convert the user's question into a SQL query. The user will ask in natural
English; produce SQL that answers the question using the relevant tables.
Return the result as SQL. Make sure the query is good.

If the question is about recent customers, use the customers table and filter
by signup date appropriately. Include anything else that is needed to make
the answer work well, and keep performance in mind.
MDEOF
git add prompts/sql-generator.md
git commit -q -m "feat: add sql generator prompt"

cat > README.md << 'MDEOF'
# Prompt Library

Team AI prompts live in `prompts/`. The SQL generator prompt is under
review; put the review in `review-report.md` at the repo root. Do not edit
the prompt file itself.
MDEOF
git add README.md
git commit -q -m "docs: add prompt library readme"

echo "Repo ready with prompts/sql-generator.md (flawed, under review)."
