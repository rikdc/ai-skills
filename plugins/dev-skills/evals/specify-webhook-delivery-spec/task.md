# Specify the Webhook Delivery Service

## Background

You are a platform engineer on the Events team of a B2B SaaS company.
Your colleague Dana, a staff engineer, has written a high-level design for
a new internal **Webhook Delivery Service**: integration partners
currently discover changes in their workspace (invoice paid, seat
invited, export finished) by polling the public API, which wastes their
quota and delays them by up to an hour. The design covers the problem,
goals, non-goals, the four components (Event Bridge, Dispatcher, Delivery
Worker, Admin API) and the key behaviours — but deliberately leaves
several details open: "the exact intervals are ours to choose", "the
exact signing scheme is ours to choose", "for how long delivery records
are kept is not decided yet".

Before the team breaks the work into tickets, Dana asks you to turn the
design into a full technical specification that developers can implement
directly. The team follows the specify skill: specs use its documented
output structure (Overview with Purpose/Scope/Success Criteria, FR-id
functional requirements with Input/Processing/Output/Edge Cases/Validation,
quantified non-functional requirements, architecture and data models,
complete API contracts with error codes, integration points, testing
strategy, deployment & operations, and Open Questions & Assumptions), are
precise rather than vague, and every requirement is testable.

Run the setup script to prepare the working directory:

```bash
bash setup.sh
```

It creates a small git repository with Dana's design committed at
`docs/webhook-delivery-design.md` on `main`. No network access is
available: if you would normally look up RFCs or standards online, skip
the lookup and reference them by name only.

## Your Task

1. Read Dana's design document in full.
2. Write `spec.md` in the repository root: a complete technical
   specification of the Webhook Delivery Service following the specify
   skill's documented output structure — every section from Overview
   through Open Questions & Assumptions.
3. Where the design leaves a detail open but expects the specification
   to decide it (retry intervals and attempt behaviour, signing scheme
   and replay protection, the consecutive-failure disabling threshold,
   payload and header formats), choose concrete values — do not leave
   them vague.
4. Where the design explicitly defers a decision (retention duration for
   delivery records, whether customers can replay failed deliveries
   themselves), record it under Open Questions & Assumptions rather than
   silently resolving it.
5. Review your specification against the skill's specification quality
   checklist and fix anything that fails before finishing.

## Output Specification

After you finish, the repository should contain:

- `spec.md` — the full technical specification, in the repository root
- `docs/webhook-delivery-design.md` — unchanged, as committed by setup
