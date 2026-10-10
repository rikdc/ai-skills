#!/usr/bin/env bash
# Workspace: a finished PRD is in tasks/, ready for task generation.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p tasks
cat > tasks/prd-invoice-matching.md << 'EOF'
# PRD: Supplier Invoice Matching

## Introduction
Finance spends hours cross-checking supplier invoices against purchase
orders by hand. We propose an invoice-matching service that ingests a
supplier invoice PDF, extracts the line items, matches them against an
approved purchase order, and flags mismatches for a finance user to resolve.

## Goals
- Reduce manual matching time by 80%.
- Flag any invoice whose total deviates from its PO by more than 2%.

## User Stories
- As a finance user, I want invoices matched against POs automatically so
  that I only review mismatches.
- As a finance user, I want a mismatch report so that I can chase suppliers.

## Functional Requirements
1. The system must accept a supplier invoice PDF upload.
2. The system must extract invoice number, date, and line items from the PDF.
3. The system must match extracted line items against the referenced PO.
4. The system must flag invoices whose total deviates from the PO by more
   than 2% and queue them for human review.
5. The system must expose a daily mismatch report over HTTP.

## Non-Goals (Out of Scope)
- Automatic payment release.
- Mobile apps.

## Success Metrics
- Manual matching hours drop to 20% of current.
- Mismatch review completed within one business day.

## Open Questions
- Which PDF extraction library should we license?
EOF

echo "PRD present at tasks/prd-invoice-matching.md."
