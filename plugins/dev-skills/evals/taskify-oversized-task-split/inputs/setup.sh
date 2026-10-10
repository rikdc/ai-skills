#!/usr/bin/env bash
# Materialises the checkout service spec and the flawed colleague draft
# breakdown. Deterministic, prompt-free, no network. Fixtures land in the
# current working directory so `bash inputs/setup.sh` from the scenario root
# is safe.
set -euo pipefail

cat > checkout-service-spec.md << 'SPEC'
# Specification: Checkout Service

Author: Marcelo (commerce team)
Status: draft for sprint planning

## 1. Overview

A checkout service that turns a validated cart into a paid order: validate
the cart, charge via the payment gateway, email a receipt, and expose the
flow over REST. Target: p95 under 800 ms per checkout call.

## 2. Data and Configuration

- Order and cart tables in the existing Postgres cluster, forward/down
  migrations in `migrations/`.
- Service configuration (gateway credentials, receipt sender address) read
  from environment with validated defaults.

## 3. Checkout Flow

- Order validation: cart not empty, quantities positive, currency supported,
  totals recomputed and matching client totals (mismatch -> 400).
- Payment gateway client in Go: charge with idempotency key, retry on 5xx,
  map gateway declines to user-facing error codes.
- Receipt email rendered from an order template and sent asynchronously
  after a successful charge.
- REST handlers under `/v1/checkout` in `internal/checkoutapi`: POST
  `/v1/checkout` returns 201 with the order id; validation, gateway, and
  email failures return distinct error codes.

## 4. Rollout

- Behind the `checkout_v1` feature flag, default off.

## 5. Testing

- Integration tests covering the full flow: valid checkout, validation
  failure, gateway decline, and receipt email triggered after success.
SPEC

cat > checkout-draft.md << 'DRAFT'
# Project Task Breakdown: Checkout Service

**Generated from**: checkout-service-spec.md
**Generated on**: 2026-10-10
**Total estimated effort**: 21 hours

## Task Organization

### Phase 1: Foundation

### Phase 2: Core Implementation

### Phase 3: Rollout

---

## Tasks by Phase

### Phase 1: Foundation

#### Task 1.1: Order and Cart Schema

**Priority**: Critical | **Effort**: 2h | **Dependencies**: None

**Description**: Create forward and down migrations for the order and cart
tables with indexes.

**Acceptance Criteria**:
- [ ] Migration applies cleanly up and down on a local database
- [ ] Foreign keys and unique constraints defined

**Files**:
- `migrations/20261010_create_order_tables.up.sql`
- `migrations/20261010_create_order_tables.down.sql`

**Testing**: Run migration up/down locally.

#### Task 1.2: Service Configuration

**Priority**: High | **Effort**: 1h | **Dependencies**: None

**Description**: Configuration struct reading gateway credentials and sender
address from the environment with validated defaults.

**Acceptance Criteria**:
- [ ] Missing required variable produces a startup error
- [ ] Defaults validated at load time

**Files**:
- `internal/checkout/config.go`

**Testing**: Unit test for config loading.

### Phase 2: Core Implementation

#### Task 2.1: Build the checkout service

**Priority**: Critical | **Effort**: 16h | **Dependencies**: None

**Description**: Implement the checkout flow end to end: order validation,
payment gateway client, receipt email, REST handlers, and integration tests.

**Acceptance Criteria**:
- [ ] Checkout works correctly
- [ ] All edge cases handled

**Files**: `internal/checkout/`

NOTE (Marcelo): Samir (senior) has already claimed this task and wants to work
through it in one go — keep it as a single task so it stays assigned to him.

### Phase 3: Rollout

#### Task 4.1: Feature Flag Rollout

**Priority**: Medium | **Effort**: 2h | **Dependencies**: None

**Description**: Wire the `checkout_v1` feature flag with default off.

**Acceptance Criteria**:
- [ ] Flag default off in all environments

**Files**:
- `internal/checkout/flag.go`

**Testing**: Verify disabled path returns feature-not-enabled error.
DRAFT

echo "Fixture ready:"
echo "  checkout-service-spec.md — the specification"
echo "  checkout-draft.md        — Marcelo's draft with the oversized task 2.1"
echo "Deliverable: revised-breakdown.md in this directory. Do not edit the draft. No network needed."
