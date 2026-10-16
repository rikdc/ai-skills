#!/usr/bin/env bash
# Initialises a git repository containing Dana's high-level webhook
# delivery design document, which the agent must turn into a technical
# specification.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "eng@example.com"
git config user.name "Events Team"

git checkout -q -b main
mkdir -p docs
cat > docs/webhook-delivery-design.md << 'MDEOF'
# High-Level Design: Webhook Delivery Service

**Author**: Dana (staff engineer, Events team)
**Status**: Reviewed at the 2026-09-28 design sync; ready for specification

## Problem

Integration partners currently discover changes in their workspace
(invoice paid, seat invited, export finished) by polling our public API.
Polling wastes their quota and delays them by up to an hour. We will
build an internal webhook delivery service that pushes workspace events
to customer-registered HTTPS endpoints.

## Goals

- Deliver workspace events to customer-registered HTTPS endpoints.
- Customers must be able to verify that a payload genuinely came from us.
- Delivery must survive transient endpoint failures without human help.

## Non-Goals

- A customer-facing UI for managing webhook endpoints. The console team
  owns the existing settings pages; our Admin API feeds them.
- Receiving webhooks from third parties — we only send.
- Real-time delivery guarantees. Delivery is at-least-once with bounded
  retries, not a hard SLA.

## High-Level Architecture

1. **Event Bridge** — internal services already publish domain events to
   the existing message bus on topics named
   `events.<service>.<entity>.<action>`.
2. **Dispatcher** — a new consumer reads the bus, looks up the endpoints
   registered for the event's workspace, and enqueues one delivery job
   per (event, endpoint) pair.
3. **Delivery Worker** — performs the HTTP POST of the JSON payload,
   signs it, and applies the retry policy.
4. **Admin API** — endpoints to register, list and rotate webhook
   endpoints and their secrets.

## Key Behaviours

- Delivery is at-least-once: duplicates are possible and consumers must
  tolerate them.
- Retries use exponential backoff with a maximum of 5 attempts; after
  the final failure the delivery is marked failed. The exact intervals
  are ours to choose.
- Payloads are signed with a per-endpoint secret. The exact signing
  scheme and header names are ours to choose, but a customer must be
  able to reject a replayed request.
- An endpoint that fails N consecutive deliveries is automatically
  disabled and its owner is emailed. What exactly counts as N is ours to
  define.
- Delivery records (request, response, status) are stored for debugging;
  for how long is not decided yet.
- Whether customers can replay failed deliveries themselves via the
  Admin API is not decided yet.

## Data Touchpoints

- Endpoints and their secrets live in Postgres (table owned by us).
- Delivery records land in Postgres too; volume is roughly 50k
  deliveries per day at launch.
MDEOF

git add docs/webhook-delivery-design.md
git commit -q -m "docs: add high-level design for webhook delivery service"

echo "Repo ready with docs/webhook-delivery-design.md committed on main."
echo "Agent must write spec.md in the repository root."
