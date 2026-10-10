#!/usr/bin/env bash
# Workspace: task list with one just-finished sub-task not yet marked.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p tasks
cat > tasks/tasks-prd-payment-retry.md << 'EOF'
# Task list for Payment Retry

## Relevant Files

- `src/retry/scheduler.py` - Retry scheduler with backoff.
- `src/retry/scheduler.test.py` - Tests for the scheduler.
- `src/retry/store.py` - Attempt persistence.

## Tasks

- [x] 1.0 Data model
  - [x] 1.1 Define attempt table
  - [x] 1.2 Migrate schema
  - [ ] 1.3 Seed migration test data
- [ ] 2.0 Retry scheduler
  - [ ] 2.1 Implement exponential backoff on the retry scheduler
  - [ ] 2.2 Persist attempt history
  - [ ] 2.3 Escalate deadline after third failure
EOF

cat > backoff-scratch.md << 'EOF'
# Mateo's scratch — still being edited, do not clean up

backoff base 2s, cap 60s? check retry-budget vs payment rails SLA.
EOF

echo "Task list with 2.1 done-but-unmarked; scratch file present."
