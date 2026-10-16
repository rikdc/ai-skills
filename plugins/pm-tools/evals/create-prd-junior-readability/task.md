# PRD Clarity for a Junior Developer

## Background

The "alert-rules" team has a standing rule: every PRD's primary reader is a
junior developer, so requirements must be explicit, unambiguous, and free of
jargon. A previous PRD on this team was rejected in review precisely because
it buried decisions in vague prose ("the system should leverage appropriate
notification infra as needed").

You are applying the `create-prd` rule. The clarifying answers are already
collected in `answers.md`; the feature is webhook notifications when a
service's error rate exceeds a configured threshold for fifteen minutes.

**Network actions must be skipped.** Work only from the fixture file.

## Your Task

1. Read `answers.md`.
2. Generate the PRD and save it as `/tasks/prd-error-rate-alerting.md`.
3. Include the rule's required sections, with special care that the
   Functional Requirements are concrete and testable: each requirement names
   exact, unambiguous behaviour (thresholds, fifteen-minute window, webhook
   destination) with no weasel words like "as appropriate", "leverage", or
   "should consider".
4. Do not begin implementing the PRD; no code files are to be created.

## Output

- `tasks/prd-error-rate-alerting.md` — the generated PRD.
