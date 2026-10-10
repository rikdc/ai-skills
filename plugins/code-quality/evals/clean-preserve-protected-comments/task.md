# Preserve Comments That the Clean Rules Protect

## Background

The **telemetry** repository has uncommitted edits in `src/sync.py` and a
colleague ran `/clean` before showing the diff around. The clean command
removes obvious or redundant comments, but it has explicit do-not-remove
rules:

- Don't remove comments indicating a stub for future work.
- Don't remove comments that start with TODO.
- Don't remove comments if doing so would make a scope empty, like an empty
  catch block (here: the handler whose only remaining content would be the
  sentinel `pass`).
- Don't remove comments that suppress linters or formatters, like
  `# noqa: ...`.
- Comments that are genuinely redundant still go.
- Any end-of-line comment moves above the line it describes.
- Only look at the local code changes that are not yet committed.

Start with `bash setup.sh` to create the repository state.

## Your Task

1. Run `git diff` to see the uncommitted changes in `src/sync.py`.
2. Remove the redundant comment(s) only: the name-restating and the dead
   code-path narration.
3. Preserve, verbatim and in place: the TODO, the stub-for-future-work
   comment, the linter-suppression comment, and the retry-handler placeholder
   comment.
4. Move the end-of-line comment above the line it describes.
5. Keep behaviour identical; do not commit or stage anything; don't edit
   files outside the uncommitted change.

## Output Specification

- `src/sync.py`: redundant comments gone; protected comments intact;
  no end-of-line comment left on the touched line
- `python3 src/sync.py` prints the same batch line as before
- Work remains uncommitted
