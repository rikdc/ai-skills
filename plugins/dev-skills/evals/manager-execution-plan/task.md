# Plan the API Keys & Rate Limiting Rollout

## Background

You are the engineering manager for the metrics ingestion service, a small Go
service that accepts telemetry over HTTP. Today the `/ingest` endpoint is
unauthenticated and unthrottled, and after a customer spiked traffic last
week the team agreed to ship API key authentication with per-key rate
limiting. A design brief has been written and merged to `main` at
`docs/design-api-keys.md`.

Prepare the repository (this also copies the design brief into it):

```bash
bash inputs/setup.sh
```

Your job: run the manager orchestration flow for this initiative — the
equivalent of invoking `/manager --plan api-keys` — and produce a concrete
execution plan the team can follow.

**Note on delegation**: no sub-agents, network calls, or real GitHub/Git
pushes are available in this exercise. Skip them. Represent every delegation
the manager would make as written assignment lines in the plan
("assign X to skill Y, input Z, expected output W"); nothing needs to
actually be sent anywhere.

## Your Task

1. Run `bash inputs/setup.sh` to prepare the repository.
2. Read `docs/design-api-keys.md` and skim the existing service code.
3. Act as the manager skill for this initiative: analyze the task, break it
   into phases and tracks, map dependencies, and match each piece of work to
   the right specialist skill (specify, taskify, go-backend-implementor, go-review,
   mentor, document).
4. Write the resulting plan to `execution-plan.md` at the repository root,
   following the manager skill's execution plan structure: task analysis,
   phased breakdown with skill assignments, dependency mapping between
   tracks, and success criteria.

## Output Specification

After you finish, the repository should contain:

- `execution-plan.md` — the orchestration plan covering all work in the
  design brief, with skill assignments per phase, parallel tracks and their
  dependencies, and success criteria
- no other modifications to the repository
