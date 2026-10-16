#!/usr/bin/env bash
# Workspace: nested PRD plus a stale unrelated task file that must survive.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p tasks

cat > tasks/prd-spare-parts-tracker.md << 'EOF'
# PRD: Spare Parts Tracker

## Introduction
Workshop tool boards run out of consumable spare parts unnoticed. A simple
tracker records stock levels and raises alerts when parts fall below
reorder thresholds.

## Goals
- Eliminate surprise out-of-stock events for the top 20 consumables.

## User Stories
- As a workshop technician, I want low-stock alerts so that I reorder
  before a job is blocked.

## Functional Requirements
1. The system must record stock levels for consumable spare parts.
2. The system must let a user set a reorder threshold per part.
3. The system must raise an alert when stock falls below the threshold.

## Non-Goals (Out of Scope)
- Automated purchasing.

## Success Metrics
- Zero blocked jobs due to missing parts per quarter.

## Open Questions
- How are thresholds seeded during onboarding?
EOF

cat > tasks/tasks-todo.md << 'EOF'
# Old effort (thermostat firmware) — do not touch

- [ ] flash firmware v1.2 to bench board
- [ ] verify PID loop tuning
EOF

echo "Nested PRD and stale tasks-todo.md in place."
