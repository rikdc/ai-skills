#!/usr/bin/env bash
# Materialises the search service specification the agent must decompose.
# Deterministic, prompt-free, no network. Fixtures land in the current
# working directory so `bash inputs/setup.sh` from the scenario root is safe.
set -euo pipefail

cat > search-service-spec.md << 'SPEC'
# Specification: In-App Document Search Service

Author: Priya (platform team)
Status: draft for implementation planning

## 1. Overview

We are adding full-text search over workspace documents. Documents live in
Postgres today; search must cover title and body text, return ranked results
in under 300 ms for the 95th percentile at 50 QPS, and be usable by both the
web app and future integrations.

## 2. Data Layer

- New Postgres tables: `documents` (id, workspace_id, title, body, updated_at)
  and `search_index` (document_id, tokens, tsvector column).
- Indexes on workspace_id and the tsvector column; foreign key from
  search_index to documents.
- Forward and down migrations, following the existing naming convention in
  `migrations/`.

## 3. Repository Layer

- Go repository package `internal/searchrepo` with interfaces for document
  lookup and index upsert. Must be mockable for tests.

## 4. Indexing Pipeline

- A tokenizer that splits title and body into searchable tokens (unicode-aware,
  lowercased, stop-word trimmed).
- An indexer worker that consumes document-update events and keeps
  `search_index` in sync, with idempotent re-indexing on retry.

## 5. Search API

- REST endpoints under `/v1/search`:
  - `GET /v1/search?q=...&workspace=...` returning ranked results with
    document id, title, and a snippet.
  - Empty or over-long queries (max 256 chars) return 400 with an error body.
- Handlers live in `internal/searchapi`.

## 6. Access Control and Limits

- API key validation middleware; invalid keys get 401.
- Per-key rate limiting (100 requests/minute) returning 429 with a
  `Retry-After` header.

## 7. Observability

- Request metrics (count, latency histogram) and structured request logging
  for the search endpoints.

## 8. Documentation

- `docs/SEARCH_API.md` describing endpoints, parameters, error codes, and
  rate-limit behaviour.

## 9. Testing and Rollout

- Unit tests for tokenizer, repository, and handlers.
- Integration tests: index-then-search round trip, rate limit behaviour,
  auth failures.
- Rollout behind the `search_v1` feature flag; flag default off.
SPEC

echo "Fixture ready:"
echo "  search-service-spec.md — the specification to decompose"
echo "Deliverable: task-breakdown.md in this directory. No network needed."
