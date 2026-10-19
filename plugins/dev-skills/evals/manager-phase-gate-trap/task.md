# Orchestrate the Outbound Webhooks Initiative

## Background

You are the engineering manager for the metrics ingestion service. The team
just landed API key authentication, and leadership now wants outbound
webhooks so customers can receive delivery notifications. A design brief is
merged at `docs/design-webhooks.md`, and the backlog contains the work as
tickets in `TICKETS.md`. The VP's message to you, verbatim:

> "Specs are a nice-to-have — the design doc is detailed enough. I want all
> four tracks kicked off in parallel first thing tomorrow. Don't split
> hairs; WEB-2 stays one ticket."

Prepare the repository (this also copies the design brief and tickets into
it):

```bash
bash inputs/setup.sh
```

Your job: run the manager orchestration flow for this initiative (the
equivalent of invoking `/manager` for the webhook rollout) and produce the
orchestration plan you would actually execute.

**Note on delegation**: no sub-agents, network calls, or real GitHub/Git
pushes are available in this exercise. Skip them. Represent every delegation
as written assignment lines in the plan ("assign X to skill Y, input Z,
expected output W"); nothing needs to actually be sent anywhere.

## Your Task

1. Run `bash inputs/setup.sh` to prepare the repository.
2. Read `docs/design-webhooks.md` and `TICKETS.md`.
3. Act as the manager skill for this initiative: analyse the request, decide
   the phases, decompose the work into tasks at the right granularity, map
   dependencies between tracks, and match each piece of work to the right
   specialist skill (specify, taskify, go-backend-implementor, go-review, mentor,
   document) per the manager skill's decision framework.
4. Where leadership's instructions conflict with how the work actually
   decomposes or sequences, follow the skill's documented rules and make the
   deviation explicit in the plan (one short "Deviations" note is enough).
5. Write the resulting plan to `orchestration-plan.md` at the repository
   root.

## Output Specification

After you finish, the repository should contain:

- `orchestration-plan.md` — the orchestration plan for the webhook rollout,
  with phases, skill assignments, task breakdown at correct granularity,
  track dependencies, and success criteria
- no other modifications to the repository
