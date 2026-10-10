# Design Brief: Outbound Webhook Delivery

## Problem

Customers want push notifications when their ingest quota is exceeded or
keys are revoked, instead of polling. We ship outbound webhooks.

## Scope

Included: signed webhook delivery with retries, delivery metrics, an audit
log of delivery attempts, admin endpoints and a minimal UI page to view and
manually retry deliveries.
Excluded: customer-editable endpoint dashboards, webhook payload
customisation, third-party event bus integration.

## Constraints

- Deliveries must be signed (HMAC) and retried with backoff.
- Metrics and the delivery worker must not share a process.

## Components

1. **Retry queue store** — Postgres table `webhook_deliveries` (id,
   endpoint, payload, attempts, next_attempt_at, status) plus repository.
   Everything else that touches deliveries depends on this existing first.
2. **Delivery worker** — consumes the retry queue, signs payloads, posts to
   customer endpoints, updates queue rows. Depends on the retry queue
   store.
3. **Delivery metrics** — counters for attempts, successes, failures,
   exported from the queue. Depends on the retry queue store (reads it);
   independent of the worker.
4. **Audit log** — record of every delivery attempt for support. Depends on
   the delivery worker emitting attempts.
5. **Admin API + UI** — list deliveries, view attempt detail, trigger
   manual retry. Depends on the retry queue store; depends on the audit log
   for attempt detail.

## Dependencies summary

- Retry queue store → (delivery worker, delivery metrics, admin API)
- Delivery worker → audit log → admin API
