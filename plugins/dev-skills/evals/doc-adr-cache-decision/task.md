# ADR-002: Cache Backend for Session State

## Background

You are working on the notifications web service (`svc-notifications`). The
team has been asked to pick a cache backend for session state before the
next milestone, and to record the decision as an Architecture Decision
Record so future hires can see why it was made. The repository follows the
house convention set by `docs/adr/ADR-001-use-postgres-for-primary-store.md`:
an ADR has **Status/Date** metadata, a *Context and Problem Statement*
section, *Decision Drivers*, a *Decision Outcome* naming the chosen option,
and *Consequences* split into positive **and** negative.

Run the setup script to prepare the repository in your working directory:

```bash
bash setup.sh
```

This initialises the repository with the current session cache
implementation, an existing ADR showing the house template, and an
engineering memo listing the candidate options. Read the code, the existing
ADR and the memo before writing anything — the decision must be grounded in
what the repository actually says about deployment and behaviour.

There is nothing to install or deploy in this scenario: no network access
is needed and no infrastructure may be provisioned. The deliverable is the
decision record only.

## Your Task

1. Run `bash setup.sh` and read `README.md`, `src/session_cache.py`,
   `docs/adr/ADR-001-use-postgres-for-primary-store.md` and
   `notes/cache-options.md` to gather the context.
2. Identify the audience for the ADR (engineering team and future
   contributors) and confirm the format is an ADR per the house template.
3. Write the decision record at
   `docs/adr/ADR-002-use-redis-for-session-cache.md`, following the same
   template as ADR-001:
   - a heading of the form `ADR-002: <decision>`,
   - `**Status**` and `**Date**` metadata lines,
   - a *Context and Problem Statement* section grounded in the current
     implementation and the deployment facts in the README,
   - a *Decision Drivers* list (at least three drivers),
   - a *Decision Outcome* section naming exactly one of the candidate
     options from the memo,
   - a *Consequences* section with both **Positive** and **Negative**
     lists.
4. Choose the option whose properties satisfy the decision drivers. Do not
   pick an option and then ignore a driver it violates — if your chosen
   option has a genuine downside, it belongs in the negative consequences.
5. Reference the related ADR-001 so readers can trace the data-store
   decision this one builds on.

## Output Specification

After you finish, the repository should contain:

- `docs/adr/ADR-002-use-redis-for-session-cache.md` — the completed decision
  record following the ADR-001 template.
