# Design Brief: API Key Authentication and Per-Key Rate Limiting

## Problem

`/ingest` currently accepts any request. We need API keys for customers and
a per-key rate limit so one customer cannot starve the others.

## Scope

Included: key issuance/revocation admin endpoints, request authentication
middleware, per-key token-bucket rate limiting, tests, documentation.
Excluded: billing integration, self-service portal, multi-region key
replication.

## Constraints

- Postgres is the only datastore available; no Redis.
- Must ship behind an env flag so we can roll out per-customer.

## Components

1. **Key store** — Postgres table `api_keys` (id, customer, key_hash,
   rate_limit, created_at, revoked_at) plus a repository type wrapping it.
2. **Key service** — hashing of presented keys, lookup, revocation
   checks. Depends on the key store.
3. **Rate limiter** — per-key token bucket, in-process. Depends on the key
   service returning the key's configured limit.
4. **Auth middleware** — reads `Authorization: Bearer <key>`, validates via
   the key service, enforces the rate limiter, attaches the customer to the
   request context. Depends on the key service and rate limiter.
5. **Admin HTTP handlers** — `POST /admin/keys` and
   `DELETE /admin/keys/{id}` for issuance and revocation. Depend on the
   middleware being in place for the admin routes themselves.
6. **Integration tests** — end-to-end auth + limit behaviour against a
   throwaway Postgres. Depend on the key store and key service.
7. **Documentation** — API reference for the admin endpoints and a runbook
   section for the rollout flag. Written last, after review.
