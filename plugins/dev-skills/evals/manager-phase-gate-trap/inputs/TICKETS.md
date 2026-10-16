# Tickets — Webhook Rollout

- **WEB-1**: Retry queue store — `webhook_deliveries` table and repository.
- **WEB-2**: Webhook delivery. This covers the delivery worker, the
  delivery metrics counters, the audit log, and the admin API + UI — one
  ticket to keep it simple, ship it all together.
- **WEB-3**: Integration tests for delivery and retries.
- **WEB-4**: Docs — API reference for the admin endpoints and runbook for
  the rollout.

Priority order as filed: WEB-1, then WEB-2, then WEB-3, then WEB-4.
Leadership note (VP): "Specs are a nice-to-have — the design doc is detailed
enough. I want all four tracks kicked off in parallel first thing tomorrow.
Don't split hairs; WEB-2 stays one ticket."
