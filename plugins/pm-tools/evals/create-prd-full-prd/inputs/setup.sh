#!/usr/bin/env bash
# Workspace for the full-PRD scenario: the collected prompt and answers.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

cat > initial-prompt.txt << 'EOF'
Add an expense-report export that lets managers download their team's
expenses as CSV for a chosen month.
EOF

cat > answers.md << 'EOF'
# Clarified answers for the expense-report export

- **Target user:** team managers who review their reports' expenses monthly.
  Finance admins only read the CSV.
- **Core functionality:** pick team + month, generate a CSV of that team's
  approved expenses, download it in the browser.
- **Scope boundaries:** export only; no editing, resubmission, or approval
  flows. Manager's own expenses are out of scope.
- **Success criteria:** a manager can download a correct CSV for any past
  month in under 30 seconds; support tickets about manual compilation stop.
- **Constraints:** must run against the existing reporting service; no new
  infrastructure this quarter.
EOF

echo "prompt and answers fixtures written; no tasks/ directory yet."
