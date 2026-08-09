# Fish Shell Review Guide

Fish is **not POSIX-compatible** and diverges from Bash/Zsh in ways that
make Bash idioms actively wrong if copied uncritically. ShellCheck does
not support Fish at all — this file, plus manual inspection and `fish -n`
(syntax-only check), is the entire toolchain for Fish review. There is no
Fish equivalent of the Google Shell Style Guide; conventions below are
drawn from Fish's own documentation and community-established practice
(fish-shell GitHub issue discussion), not a single ratified standard —
treat style (not correctness) findings here as advisory.

## Variables are always lists

Every Fish variable is a list, even a "scalar":

```fish
set name Alice          # a one-element list
set names Alice Bob Cy  # a three-element list
echo $name[1]            # 1-indexed, not 0-indexed
echo $names[2]            # Bob
echo $names[-1]           # Cy — negative indices count from the end
```

**Review implication**: code that assumes `$var` is always a single value
(e.g., using it directly in a numeric comparison without considering it
could be empty or multi-element after a command substitution) is a
correctness risk. `set x (some command)` captures each output line as a
separate list element, not a single string — check whether that's
actually what the author wants.

## No `errexit` — you must check status explicitly

**This is the single most important Bash-to-Fish trap.** Fish has no
`set -e` equivalent for "exit the script on any command failure."
`set -e VARNAME` in Fish means **erase this variable** — it has nothing to
do with error handling. A script author porting Bash strict-mode habits
and writing `set -e` at the top of a Fish script has written a no-op (or
a bug, if a variable named that way happens to exist).

Correct patterns for error handling in Fish:

```fish
# Check $status (fish's equivalent of $?) explicitly
mkdir /some/dir
if test $status -ne 0
    echo "mkdir failed" >&2
    exit 1
end

# Or use and/or chaining
mkdir /some/dir; and cd /some/dir; or exit 1
```

**Review checklist**: for any script doing multiple sequential operations
where a failure should stop execution, verify each critical step either
checks `$status` or is chained with `and`/`or` — flag scripts that assume
Bash-style implicit errexit.

## Scope flags, not `local`/`export`

Fish variable scope is set at declaration time via flags to `set`, not via
separate `local`/`export`/`global` keywords:

| Flag | Scope |
|------|-------|
| `-l` / `--local` | Current block (function or `begin...end`) |
| `-f` / `--function` | Entire enclosing function, even in nested blocks |
| `-g` / `--global` | Global to the current shell session |
| `-U` / `--universal` | Persisted across all sessions for the user (survives restarts) |
| `-x` / `--export` | Exported to the environment of child processes (combine with a scope flag, e.g. `-lx`) |

**Review checklist**: functions that write to variables with no scope flag
(bare `set var value`) default to function scope if the variable doesn't
already exist in an outer scope, or overwrite the outer variable if it
does — this is a common source of accidental global mutation. Prefer
explicit `-l` inside functions. Flag `-U` (universal) used for anything
that isn't genuinely meant to persist across terminal sessions — it's easy
to reach for by mistake and creates surprising state that outlives the
script.

## Quoting and word-splitting differences

- Unquoted variable expansion of a list spreads each element as a separate
  argument — similar to Bash's `"$@"` — but Fish **does not** perform
  IFS-style word-splitting or glob-expand the *contents* of a variable the
  way Bash does. A value containing a space stays one list element unless
  it came from a multi-line command substitution.
- Command substitution `(cmd)` splits output into a list **by line** by
  default. Wrap in `string collect` (or use `(cmd | string collect)`) when
  the output must be treated as a single string rather than split.
- Quoting a variable (`"$var"`) forces it to behave like a single string,
  joining list elements with spaces — this is closer to Bash's `"$*"` than
  `"$@"`. Getting quoted vs. unquoted backwards is the Fish analogue of a
  Bash missing-quotes bug: it changes whether downstream commands see one
  argument or several.
- Globs (`*`, `**`) are expanded when they appear literally in a command;
  they are **not** re-expanded out of variable contents, which removes a
  whole class of Bash injection-via-unquoted-glob risk — but don't treat
  this as blanket safety; validate input before using it in filesystem
  operations regardless.

## Avoid Bash syntax leakage

Common tells that Bash habits leaked into a Fish script (all invalid or
wrong in Fish):

- `if [ ... ]; then` / `fi` — Fish uses `if ... ; end` (no `then`,
  closing keyword is `end` for every block, not `fi`/`done`/`esac`).
- `$(cmd)` — Fish uses `(cmd)` for command substitution (no `$`).
  `$(cmd)` will not do what the author expects.
- `export VAR=value` — Fish uses `set -gx VAR value`.
- `function ... { ... }` with braces — Fish functions are
  `function name ... end`, no braces.
- `$?` — Fish uses `$status`.
- `&&` / `||` — valid in modern Fish as of a few releases, but the
  idiomatic and universally-supported form is `and`/`or`. Flag as a
  portability note (not a hard error) if the script targets older Fish.

## Style conventions (community-established, not official)

No canonical style guide exists for Fish. Discussion in the fish-shell
issue tracker (github.com/fish-shell/fish-shell#1838, #2703) has
converged informally on:

- 4-space indentation.
- `snake_case` for function and variable names.
- ~80-character line length as a soft target.
- Prefer Fish builtins (`string`, `math`, `path`) over shelling out to
  external `sed`/`awk`/`bc`/`expr` for simple text/number operations.
- Wrap reusable logic in functions rather than repeating command
  sequences; put shared functions under `functions/` for autoloading if
  the script is part of a larger Fish configuration.

Treat violations of these as **Low severity style** findings, distinct
from the Fish correctness issues above (`set -e` confusion, missing
`$status` checks, scope-flag mistakes), which should be scored as
correctness/robustness findings per `SKILL.md`.

## Available tooling

- `fish -n script.fish` — syntax-only check (parses without executing).
  This is the closest Fish equivalent to a linter available by default;
  it catches parse errors but not the semantic issues above.
- No direct Fish equivalent of `shfmt` is in common use; formatting
  consistency must be checked manually against the conventions above.
