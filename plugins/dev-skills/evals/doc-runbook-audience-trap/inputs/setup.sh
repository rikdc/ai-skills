#!/usr/bin/env bash
# Initialises the notification-service repository: developer README (the
# bait for the wrong doc type), notifier source with the real health
# endpoint, and an ops incident summary with the three recurring pages.
# The agent must look past the API-centric README and choose a runbook.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

git checkout -q -b main

cat > README.md << 'MDEOF'
# notification-service

Queued email and push notifications for the platform.

## API

### POST /notify
```json
{ "channel": "email", "template": "welcome", "user_id": 42 }
```

Returns `202 Accepted`. Invalid channel returns `400`.

### GET /healthz
Returns `200 OK` when the process is up.

## Local development

```bash
pip install -r requirements.txt
make run
```

Owner: **platform-team** (#platform-team Slack, platform-team@example.com).
MDEOF

mkdir -p src ops docs
cat > src/notifier.py << 'PYEOF'
"""Notification worker and API."""
import json
import smtplib


def send_email(to: str, template: str) -> None:
    """Send an email via the SMTP provider; raises on provider timeout."""
    with smtplib.SMTP("smtp.provider.example.com", 587, timeout=10) as s:
        s.sendmail("noreply@example.com", [to], render(template, to))


def enqueue(message: dict) -> str:
    """Push a message onto the notification queue."""
    # Real deployment uses a managed queue; retries are capped at 5.
    return json.dumps(message)


def is_healthy() -> bool:
    """GET /healthz is served from this check."""
    return True
PYEOF

cat > ops/incident-summary.md << 'MDEOF'
# Recurring pages, last quarter

Summary of pages on the notification-service rotation.

1. **Worker backlog spikes** after marketing bulk sends. Diagnosis: check
   queue depth metric. Resolution: scale worker replicas up manually; the
   queue drains in ~10 minutes.
2. **SMTP provider timeouts** (`smtplib.SMTPTimeoutError`). Diagnosis:
   correlate error logs with provider status page. Resolution: fail over to
   the secondary SMTP relay listed in the address book.
3. **Queue messages stuck in retry**. Diagnosis: inspect dead-letter
   counters; a stuck message has retried 5 times. Resolution: replay the
   dead-letter queue back onto the main queue.

Contact for all three: platform-team on-call (PagerDuty schedule
"platform-notifications").
MDEOF

cat > requirements.txt << 'EOF'
fastapi==0.115.0
EOF

git add -A
git commit -q -m "chore: notification service README, worker source and ops incident summary"

echo "Repo ready on main: README.md, src/notifier.py, ops/incident-summary.md."
