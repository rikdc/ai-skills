# Bash Style Guide (condensed from Google Shell Style Guide)

Source of truth: https://google.github.io/styleguide/shellguide.html — cite
this when a finding is a style violation rather than deriving rules ad hoc.

## When shell is (and isn't) the right choice

- Shell is acceptable for small utilities and simple wrapper scripts.
- If a script is **primarily calling other utilities with little data
  manipulation**, shell is fine even if it's longer.
- If the script exceeds roughly **100 lines**, or starts using non-straightforward
  control flow / data structures, **rewrite it in a structured language**
  (Python, Go, etc.) instead of continuing to grow it. Flag scripts near or
  over this threshold as a maintainability finding, not a hard blocker.
- Never write performance-critical code in shell.

## File structure

- Executables should have a `.sh` extension or none at all (bare name);
  libraries meant to be sourced **must** have a `.sh` extension and **must
  not** be executable, per Google's guide. (Some codebases instead use a
  `.bash` extension for sourced-only files to make "this is a library, not
  an entry point" visible at a glance — either is fine as long as the
  project is consistent; follow the project's own stated convention over
  this default when one exists.)
- Start every executable with `#!/bin/bash` (not `/bin/sh`, not `env bash`
  unless the environment truly requires PATH-based lookup) and minimal flags
  on the shebang line.
- Immediately after the shebang, put a top-of-file comment describing the
  script's purpose.
- Use `set -o pipefail` near the top. **Don't** reach for `set -e`/`set -u`
  as a blanket safety net — this skill treats their presence as a finding,
  not their absence; see [[bash-pitfalls]] for why, and use explicit
  per-command exit-status checks instead.
- If the script has more than one function, define a `main` function and
  call `main "$@"` as the **last line of the file**. This keeps top-level
  execution order obvious and testable.
- For a script meant to be both executable *and* sourceable (so its
  functions can be unit-tested by sourcing the file without triggering
  `main`), guard the `main` call:

  ```bash
  main() {
    ...
  }

  if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
  fi
  ```

## Formatting

- **2-space indentation**, no tabs.
- Maximum line length **80 characters** where reasonably achievable.
- `; then` and `; do` go on the **same line** as `if`/`for`/`while`:

  ```bash
  if [[ -f "${file}" ]]; then
    ...
  fi

  for arg in "$@"; do
    ...
  done
  ```

- Pipelines that don't fit on one line: one segment per line, `|` at the end
  of the line, continuation lines indented.
- Use `[[ ... ]]` in preference to `[ ... ]` or `test` — it doesn't
  word-split or glob its operands and supports `&&`/`||`/`=~` directly.
- Use `(( ... ))` for numeric comparisons, not `[[ $x -gt 10 ]]`:
  `(( value > 10 ))` reads closer to the comparison it performs and avoids
  the `-gt`/`-lt`/`-eq` operator vocabulary entirely.
- Prefer `$(command)` for command substitution; never use backticks
  (nesting and escaping are error-prone).

## Quoting

- **Quote all expansions**: `"${var}"`, `"$1"`, `"$(command)"`. Unquoted
  expansions are the single most common source of word-splitting and
  globbing bugs (ShellCheck SC2086 — see [[shellcheck-codes]]).
- Prefer `"${var}"` over `"$var"` for clarity, especially when adjacent to
  other text.
- Use `'single quotes'` for literal strings with no expansion, `"double
  quotes"` for strings needing variable or command expansion.
- Arrays for anything that is conceptually a list, especially argument
  lists to be passed to another command — never build a single
  space-joined string and rely on word-splitting to re-separate it.

  ```bash
  # Good
  local -a flags=(--verbose --output "${outfile}")
  some_command "${flags[@]}"

  # Bad: relies on word-splitting, breaks on paths with spaces
  local flags="--verbose --output ${outfile}"
  some_command ${flags}
  ```

## Heredocs and multi-line output

- Prefer a tab-indented here-doc over a run of `echo`/`printf` calls for
  multi-line output — one block instead of N statements:

  ```bash
  # Good
  cat <<- EOF
  	Something
  	${variable}
  	EOF

  # Avoid
  echo "Something"
  echo "${variable}"
  ```

  `<<-` strips **leading tabs** (not spaces) from the body and the closing
  marker, which is what allows the here-doc to be indented with the
  surrounding code.
- Quote the marker (`<<- 'EOF'`) when the body must **not** have variables
  or command substitutions expanded — e.g. emitting literal `$` text.
- If tab-based indentation doesn't work for the context, fall back to an
  unindented here-doc (`<< EOF`, no dash, no leading tabs), or redirect a
  whole `{ ...; }` block when a here-doc doesn't fit the shape of the
  output:

  ```bash
  {
    echo "Something"
    echo "Something else"
  } >> "${GITHUB_OUTPUT}"
  ```

## Idioms

- **Regex as a separate variable before `=~`**: assign the pattern to a
  variable first rather than inlining it, to sidestep quoting ambiguity
  (ShellCheck SC2076 — quoting the right-hand side of `=~` silently turns
  it into a literal match) and version-dependent parsing differences in
  older Bash:

  ```bash
  # Good
  local re='^[0-9]+$'
  if [[ "${var}" =~ ${re} ]]; then

  # Avoid
  if [[ "${var}" =~ ^[0-9]+$ ]]; then
  ```

- **Bash built-ins over external commands** where one exists — `[[ =~ ]]`
  instead of piping to `grep`, parameter expansion (`${var//x/y}`) instead
  of `sed` for simple substitutions (ShellCheck SC2001). Fewer subprocess
  spawns, and the logic stays inline instead of round-tripping through a
  pipe.
- **Prefer long-form flags on invoked commands** (`jq --raw-output` over
  `jq -r`) when the tool supports them — readability at the call site over
  brevity. This is a house-style preference, not a correctness issue: flag
  it at most Low severity, and don't apply it to flags so common they're
  effectively their own vocabulary (`rm -f`, `ls -la`).

## Naming

- **Functions and local variables**: lowercase with underscores,
  `my_func`, `my_var`. Prefer `local` for all function-scoped variables.
- **Constants and environment/exported variables**: uppercase with
  underscores, `readonly MAX_RETRIES=3`, declared at the top of the file
  right after the header comment.
- **Package/library-style filenames**: lowercase with underscores.
- Function names use `::` as a namespace separator for libraries only when
  needed to avoid collisions (`mypackage::my_func`); don't invent this for
  a single-purpose script.

## Comments

- File header comment states the script's purpose.
- Function header comments describe purpose, globals used/modified, args,
  and outputs/return value for any non-trivial function — mirror the "why",
  not a restatement of the code.

## Common structural smells to flag

- Missing `main`/`main "$@"` pattern in a script with 3+ functions.
- Deeply nested conditionals that would read better as early returns
  (`continue`/`return` guard clauses).
- Global mutable state read/written from many functions instead of being
  passed as arguments.
- A script that has clearly outgrown shell (heavy string/data manipulation,
  JSON parsing beyond `jq` one-liners, anything needing real data
  structures) — recommend a rewrite rather than further shell patches.
