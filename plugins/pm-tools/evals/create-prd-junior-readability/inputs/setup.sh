#!/usr/bin/env bash
# Workspace for the readability-guard scenario.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

cat > answers.md << 'EOF'
# Clarified answers for error-rate alerting

- **Problem:** engineers miss outages because nobody watches dashboards
  continuously; the pager only fires on total service death.
- **Target user:** on-call engineer for the service.
- **Core functionality:** when the error rate of the monitored service stays
  above the configured threshold (default 5%) for fifteen consecutive
  minutes, POST a webhook notification to the team's configured webhook URL
  with the service name, window, and current rate.
- **Scope boundaries:** webhook delivery only; no email/SMS, no dashboard
  UI changes, no auto-remediation.
- **Success criteria:** <10 false alerts per month per service; recovery
  notifications mention the window that triggered.
- **Constraints:** must reuse the existing metrics scrape every one minute;
  webhooks fire only once per incident, not per minute.
EOF

echo "answers fixture written; no tasks/ directory yet."
