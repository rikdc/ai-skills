# Bash Pitfalls and Strict-Mode Guidance

Primary source for the pitfalls below: https://mywiki.wooledge.org/BashPitfalls
— treat it as the canonical secondary reference alongside ShellCheck
(`shellcheck-codes.md`) for Bash-specific runtime bugs that static analysis
sometimes under-explains.

## High-value pitfalls to check for

- **`for f in $(ls ...)` / any command-substitution loop over filenames.**
  Word-splits on whitespace and glob-expands the result — mangles
  filenames with spaces, globs, or newlines. Use a glob (`for f in *.mp3`)
  or `find ... -print0 | while IFS= read -r -d '' f`. (ShellCheck SC2045,
  SC2044.)

- **Unquoted parameter/command-substitution expansions everywhere**
  (`$var`, `$(cmd)`, `$@`). This is the single most common source of bugs
  in the wild — quote by default, only leave something unquoted with a
  comment explaining why splitting is intentional. (ShellCheck SC2086,
  SC2046, SC2068.)

- **`cd` without a failure check.** An unhandled failed `cd` leaves the
  rest of the script running in the wrong directory — dangerous right
  before any `rm`, `mv`, or write. Always `cd dir || exit` (or `|| return`
  inside a function). (ShellCheck SC2164.)

- **Exporting `CDPATH`.** An exported `CDPATH` changes where a script's own
  `cd` calls land, and can leak into command substitution output in older
  shells. Never export it in a script; unset it defensively if the script
  must be robust against a polluted environment: `unset CDPATH` near the
  top.

- **`cmd1 | while read ...`** — the loop body runs in a subshell (in
  Bash), so variables it sets or side effects it has (e.g., incrementing a
  counter) vanish once the pipeline exits. Use process substitution
  instead: `while read -r line; do ...; done < <(cmd1)`.

- **Reading input without `IFS=` and `-r`.** Plain `read line` trims
  leading/trailing whitespace per the default `IFS` and mangles
  backslashes. Use `IFS= read -r line` when the line must be preserved
  exactly.

- **Locale-dependent multibyte handling in `read -d ''`.** Under certain
  locales, null-delimited reads can behave unexpectedly with multibyte
  characters. Prefer `LC_ALL=C` for byte-oriented parsing loops that must
  be locale-independent.

- **`[ ]` with unquoted operands** (`[ $x = y ]`). If `$x` is empty or
  contains spaces, this either throws a "unary operator expected" error or
  silently misparses. Use `[[ $x = y ]]` (no splitting/globbing inside
  `[[ ]]`) or quote inside `[ ]`: `[ "$x" = y ]`.

- **Array element expansion in destructive contexts** (e.g., array index
  or values used inside `rm`, `eval`, or arithmetic contexts) without
  validating they're actually numeric/expected — this is a functional
  injection risk when the array is populated from external input. See
  `security-checklist.md`.

- **Redirection order.** `cmd 2>&1 >>logfile` does **not** redirect stderr
  to the log file — redirections apply left to right, and at the point
  `2>&1` runs, `1` is still the terminal. Use `cmd >>logfile 2>&1`.

- **Backticks instead of `$(...)`.** Nesting and escaping inside backticks
  is genuinely broken in edge cases; `$(...)` is strictly better and has
  been portable for well over a decade. (ShellCheck SC2006.)

## Error handling: explicit checks over `set -e`/`set -u`

A widely-cited pattern is the "unofficial Bash strict mode" header:

```bash
set -euo pipefail
IFS=$'\n\t'
```

**This skill does not recommend `-e` (errexit) or `-u` (nounset) as a
baseline for every script.** Keep `-o pipefail` and the `IFS` narrowing —
they're unambiguously safe defense in depth. Treat `-e`/`-u` as an
anti-pattern to flag when present, not a best practice to require when
absent, for the reasons below.

- **`-o pipefail`** — a pipeline's exit status becomes the last non-zero
  status among its stages, instead of always being the last stage's
  status. Without it, `false | true` "succeeds". Keep this.
- **`IFS=$'\n\t'`** — narrows word-splitting to newline and tab only, so
  an accidentally-unquoted expansion doesn't also split on spaces. Defense
  in depth, not a substitute for quoting. Keep this.
- **`-e` (errexit)** — exits immediately if a simple command exits
  non-zero, but its rules for *which* commands count are notoriously
  inconsistent (see below). Scripts that lean on `-e` for correctness
  develop false confidence: the header looks like error handling but
  silently no-ops on exactly the commands an author is most likely to
  wrap in a condition or a helper function.
- **`-u` (nounset)** — turns a reference to an unset variable into a
  runtime error, but only on the exact code path that happens to execute.
  It can't catch an unset-variable bug on a branch that isn't exercised in
  this run. ShellCheck (SC2086, SC2154) catches the same class of bug
  statically, across every branch, at review time — before the script
  ever runs.

### Why `-e` is unreliable

- **Does not fire inside a command substitution used in a condition**,
  e.g. `if [ "$(false)" ]` — the inner failure is swallowed because it's
  the condition of an `if`, not a standalone statement.
- **Does not fire for any command that is part of an `&&`/`||` list**, or
  the condition of `if`/`while`/`until`, or preceded by `!`. This includes
  calling a function that itself contains a "critical" failing command —
  the failure is suppressed at the call site, not just the immediate
  command.
- **Does not fire for any command in a pipeline** except, with
  `pipefail`, affecting the pipeline's overall status — and even then
  only where that overall status is actually checked.

### Why `-u` isn't a substitute for review

- **Coverage is runtime-only and path-dependent** — a bug on an untaken
  branch stays hidden indefinitely.
- **Empty arrays misbehave on Bash < 4.4**: `"${arr[@]}"` for a
  zero-element array raised a spurious unbound-variable error on older
  Bash versions, which trains authors to work around `-u` rather than
  trust it.

### What to actually flag

- **`set -e`/`set -u` present** (bare or as part of `set -euo pipefail`):
  Low/Medium **Style** finding — recommend removing `-e`/`-u`, keeping
  `-o pipefail`, and replacing whatever error handling `-e` was standing
  in for with explicit checks (below). This is a maintainability finding,
  not a safety one — don't score it above Medium by itself.
- **Any critical command with no explicit exit-status check** — `cd`,
  destructive `rm`/`mv`/`cp`, a build/deploy step whose failure should
  stop the script — is a **Correctness** finding regardless of whether
  `-e` is present, because `-e` cannot be trusted to catch it reliably
  (see above). This is the primary way this skill scores error handling,
  not "does the script have a strict-mode header."
- **Missing `set -o pipefail`/`setopt PIPE_FAIL`** on a script with a
  pipeline whose failure matters is still its own finding.

The explicit-check alternative:

```bash
cd "$deploy_dir" || exit 1
mkdir -p "$out_dir" || { echo "mkdir failed" >&2; exit 1; }
build_artifact || exit 1
```

This is more verbose than a one-line strict-mode header, but every check
is visible at the call site and behaves the same whether the failing
command is top-level, inside a function, or inside a conditional —
exactly the cases where `-e` silently does nothing.
