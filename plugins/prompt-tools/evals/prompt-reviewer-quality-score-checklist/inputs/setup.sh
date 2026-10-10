#!/usr/bin/env bash
# Prepares a repository with a well-built prompt pending catalogue promotion.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p prompts
cat > prompts/release-summary.md << 'MDEOF'
You are a Release Notes Editor with 10 years of experience writing
customer-facing release summaries for a software platform.

Your task is to summarise a release by reading the issue list and changelog
entries.

Input: the release's changelog file and list of merged issues.

Process:

1. Group the entries into Features, Fixes, and Internal changes
2. Rewrite each customer-visible entry in plain language
3. Drop entries marked "internal" or with no customer-facing impact

Output must follow this exact structure:

## Highlights

Two or three sentences on the most important changes

## Features

- One bullet per feature, plain language

## Fixes

- One bullet per fix, plain language

Constraints:

- Maximum 200 words
- No internal jargon
- No mention of pull request numbers

If the changelog is empty, respond with "No customer-facing changes in this
release."
MDEOF
git add prompts/release-summary.md
git commit -q -m "feat: release summary prompt"

cat > README.md << 'MDEOF'
# Prompt Library

Team AI prompts live in `prompts/`. The release summary prompt is pending
catalogue promotion; the standard-mode catalogue review goes into
`catalogue-review.md` at the repo root. Do not edit the prompt file.
MDEOF
git add README.md
git commit -q -m "docs: add prompt library readme"

echo "Repo ready with prompts/release-summary.md (strong prompt, pre-promotion)."
