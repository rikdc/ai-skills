# Specify the Media Upload Service Without Copying Vague Requirements

## Background

Your colleague Sam runs the docs platform. After a chaotic product review
he pasted his meeting notes into a "high-level design" document for a new
**Media Upload Service**: users attach screenshots and diagrams to
help-centre articles, and today they host images elsewhere and paste
links that break. The notes are honest about what was decided, but most
of the requirement lines are the product manager's verbatim phrasing —
"should respond quickly", "might fail sometimes — handle those
gracefully", "should maybe get cleaned up at some point", "should
probably support at least a thousand users".

Sam asks you to produce the technical specification the team will
implement from. He points at the specify skill's rules: specs are precise
(not "handle invalid input" but "return 400 Bad Request"), every
functional requirement is testable with clear pass/fail criteria,
performance targets are quantified, there is no ambiguous language
("should", "might", "probably"), and unresolved design decisions go into
the Open Questions & Assumptions section — they are never silently
settled, and certainly never silently promoted into firm requirements.

Run the setup script to prepare the working directory:

```bash
bash setup.sh
```

It creates a small git repository with Sam's notes committed at
`docs/media-upload-design.md` on `main`. No network access is available:
if you would normally look up RFCs or standards online, skip the lookup
and reference them by name only.

## Your Task

1. Read Sam's notes in full. Some lines are firm decisions; most are not.
2. Write `spec.md` in the repository root: a complete technical
   specification of the Media Upload Service following the specify
   skill's documented output structure, from Overview through Open
   Questions & Assumptions.
3. Transform every vague requirement line into a precise, testable
   requirement: concrete thresholds, exact status codes, and
   pass/fail-verifiable statements. Where the notes give a figure in an
   uncertain tone (Sam's "something like 50 MB?"), you may adopt it, but
   mark it as an assumption requiring validation.
4. Anything the notes mark as undecided (malware scanning scope, upload
   visibility) must not become an unconditional firm requirement; record
   it under Open Questions & Assumptions instead.
5. Review against the specification quality checklist and fix anything
   that fails before finishing.

## Output Specification

After you finish, the repository should contain:

- `spec.md` — the full technical specification, in the repository root
- `docs/media-upload-design.md` — unchanged, as committed by setup
