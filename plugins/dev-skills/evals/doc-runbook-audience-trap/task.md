# Document the Notification Service for the On-Call Rotation

## Background

You are on the platform team. The on-call lead has pinged you: the team
keeps getting paged for the `notification-service` at 3am, and the only
documentation that exists is the developer README, which describes the
HTTP API but nothing about incidents, health checks or escalation. The
lead's message is vague — "we need documentation for the notification
service, please" — and does not say what kind.

Run the setup script to prepare the repository in your working directory:

```bash
bash setup.sh
```

It initialises a repository containing the service README (developer
audience), the notifier source code (including its health endpoint) and an
ops incident summary listing the three recurring page causes.

Because this scenario is non-interactive, you cannot ask the lead
clarifying questions directly: work out the answers from the context and
**record them explicitly** in a short decision note (`doc-choice.md`)
before you write the document itself. The documented documentation workflow
is to identify the audience first, then choose the format that suits that
audience — choose according to whom the document must serve, not
mechanically to the artefact sitting in the repo.

Nothing may be deployed and no network access is needed. The deliverables
are the decision note and the document itself.

## Your Task

1. Run `bash setup.sh` and read `README.md`, `src/notifier.py` and
   `ops/incident-summary.md`.
2. Identify the primary audience from the context (who has the problem:
   the on-call rotation, not new developers) and write `doc-choice.md`
   stating:
   - the target audience you identified,
   - the documentation type/format you chose for that audience
     (API/OpenAPI, ADR, architecture doc, onboarding guide, or runbook),
   - why that format fits the audience,
   - why the developer-focused alternative(s) you considered and rejected
     do not fit.
3. Write the document itself following the structure appropriate for the
   chosen format, using facts from the repository:
   - service overview and ownership,
   - health checks (grounded in the endpoint the code actually exposes),
   - the common issues from the incident summary, each with diagnosis
     steps and resolution steps,
   - monitoring and metrics where the repo gives you something concrete,
   - deployment and rollback expectations,
   - contact and escalation paths.
4. Do not pad the document with developer-facing content that does not
   serve the on-call audience (e.g. local development setup, prerequisites,
   full OpenAPI schema dumps).

## Output Specification

After you finish, the repository should contain:

- `doc-choice.md` — audience identified, chosen format, fit rationale, and
  the rejected alternatives with reasons.
- The document itself in a sensible location (for example
  `docs/notification-service-runbook.md`).
