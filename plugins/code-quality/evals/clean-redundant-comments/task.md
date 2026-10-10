# Remove Redundant Comments from Uncommitted Changes

## Background

You are in the **ledger** repository, mid-change: `src/ledger.py` has
uncommitted edits riddled with observations the code no longer needs, and a
colleague asked for `/clean` on the repository before anything is staged.
The clean command's rules are as follows:

- Remove comments that are obvious or redundant: commented-out code, edit
  narration ("changed X", "removed Y"), and explanations that merely restate
  a nearby method name.
- Move any end-of-line comment above the line it describes — comments belong
  on their own lines.
- Never remove: TODO comments, stub-for-future-work comments, comments that
  would leave an otherwise-empty scope (empty except/else), or
  linter/formatter suppressors like `# noqa`.
- Only look at the local code changes that are not yet committed.

Start with `bash setup.sh` to create the repository state.

## Your Task

1. Run `git diff` to see the uncommitted changes in `src/ledger.py`.
2. Apply the clean rules to those changes: drop the redundant comments,
   move the end-of-line comment above its line, and leave the protected
   comment (TODO, stub, noqa, empty-scope fillers) exactly where it is.
3. Do not reformat unrelated code, rename anything, or change behaviour; and
   do not touch files outside what the command actually works on.
4. The module must still work exactly as before the edit.

## Output Specification

- `src/ledger.py` cleaned: redundant/unobvious comments removed
- No end-of-line comments remain on the touched lines
- `python3 src/ledger.py` still succeeds (the demo entry point prints the
  same total as before)
