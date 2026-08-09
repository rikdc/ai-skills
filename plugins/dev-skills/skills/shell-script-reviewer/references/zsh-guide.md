# Zsh Review Guide

Zsh is close enough to Bash that authors often copy Bash idioms wholesale,
but several defaults differ in ways that change both correctness and the
risk profile of "missing quotes" bugs. **ShellCheck has no native Zsh
dialect** — `run_shellcheck.sh` can be pointed at a Zsh file with
`--shell=bash` as a best-effort check of portable constructs only (see
`shellcheck-codes.md`); treat anything Zsh-specific as needing manual
review against this guide instead.

## Word-splitting differs from Bash by default

In native Zsh emulation, **unquoted parameter expansion of a scalar does
not word-split and does not glob-expand** the way it does in Bash:

```zsh
var="a b c"
for x in $var; do echo "$x"; done
# Bash: prints a / b / c  (three iterations — word split)
# Zsh:  prints "a b c"    (one iteration — no split by default)
```

This is controlled by the `SH_WORD_SPLIT` option (off by default in native
Zsh; on automatically when Zsh is invoked as `sh`/`ksh`) and `GLOB_SUBST`
(off by default — parameter-expansion results aren't re-interpreted as
glob patterns).

**Review implication — this changes but does not eliminate injection
risk**: a script relying on this default behaves safely for the classic
"unquoted variable with spaces breaks into multiple arguments" bug, but:

- The script becomes **non-portable** — the exact same unquoted code
  behaves differently if the file is ever run under `sh`, `bash`, or Zsh
  in `sh`-emulation mode. Flag unquoted expansions as a portability
  finding even when Zsh's native behavior happens to be safe.
- **`$@`, `"$@"`, positional parameters, and array expansions
  (`${array[@]}`) still split into multiple words** regardless of
  `SH_WORD_SPLIT` — those are inherently list-valued, not a scalar being
  split. Bugs here (missing quotes around `${array[@]}`) still apply.
- Command substitution `$(cmd)` still splits on whitespace/newlines by
  default in list contexts, same general shape as Bash.
- Still quote everything by default — don't let a reviewer or author use
  "Zsh doesn't split" as a reason to skip quoting; it's a portability and
  clarity issue even when not an immediate correctness bug.

## Arrays are 1-indexed by default

```zsh
arr=(a b c)
echo $arr[1]   # "a" in Zsh (vs. would be unset/empty in Bash, where
               # arr[1] is the *second* element)
```

`setopt KSH_ARRAYS` switches to 0-indexed, Bash-style arrays for
compatibility scripts. **Review checklist**: confirm the author isn't
mixing 0-indexed assumptions (common when porting from Bash) into a script
that hasn't set `KSH_ARRAYS` — an off-by-one here is silent, not an error.

## `NOMATCH` — unmatched globs abort by default

Zsh's default behavior for a glob pattern that matches nothing is to
**print "no matches found" and abort** (non-interactive scripts exit
non-zero), unlike Bash, which leaves an unmatched glob as the literal
pattern text. A script ported from Bash that relies on an unmatched glob
silently passing through as a literal string will break under Zsh.

- `setopt NO_NOMATCH` restores Bash-like "leave it literal" behavior.
- `setopt NULL_GLOB` makes an unmatched glob disappear entirely (expands
  to nothing) instead of erroring — matches Bash's `shopt -s nullglob`.
- The `(N)` glob qualifier applies null-glob behavior to a single glob
  expression without changing global shell options: `*.txt(N)`.

Flag any script using globs without considering which of these behaviors
it actually wants, especially scripts intended to be portable to/from
Bash.

## Extended globbing risks

With `setopt EXTENDED_GLOB`, Zsh glob patterns gain significant power:
`^pattern` (negation), `pattern~exclude` (exclude), `(#i)` (case
insensitive), recursive `**/`, and more. This is useful but means a
string the author thinks is a plain literal (containing `^`, `~`, `#`, or
`(`) can be silently reinterpreted as a glob expression once
`EXTENDED_GLOB` is active. Review checklist:

- Is `EXTENDED_GLOB` actually needed, or could the script use a narrower,
  explicit approach (`string.gsub`-equivalent tooling, `case` patterns)?
- Are values that might contain these special characters (filenames from
  user input, external data) ever used unquoted where extended globbing
  is active?

## Error handling: explicit checks over `ERR_EXIT`/`NO_UNSET`

This mirrors the Bash stance in `bash-pitfalls.md`: keep `PIPE_FAIL`,
avoid `ERR_EXIT`/`NO_UNSET` as a blanket header.

| Bash | Zsh | Recommendation |
|------|-----|-----------------|
| `set -o pipefail` | `setopt PIPE_FAIL` | Keep. Pipeline exit status reflects the last non-zero stage instead of always the last stage. |
| `set -e` | `setopt ERR_EXIT` | Avoid as a blanket header — the same unreliability caveats from `bash-pitfalls.md` apply identically to `ERR_EXIT` (doesn't fire inside a substituted condition, inside `&&`/`\|\|` lists, or for non-final pipeline stages without `PIPE_FAIL`). Use explicit `cmd \|\| exit`/`cmd \|\| return` at each critical step instead. |
| `set -u` | `setopt NO_UNSET` | Avoid as a blanket header, same reasoning as Bash's `-u` — it only catches unset-variable bugs on the exact path executed. **Caveat specific to Zsh**: ShellCheck's `--shell=bash` approximation is weaker coverage here than it is for native Bash, so lean more on manual review of variable usage in Zsh scripts than you would for Bash before concluding a script is safe without `NO_UNSET`. |

**What to flag**: `setopt ERR_EXIT`/`NO_UNSET` present → Low/Medium Style
finding recommending removal in favor of explicit checks. Any critical
command (`cd`, destructive file ops, a deploy step) with no explicit
exit-status check → Correctness finding regardless of `ERR_EXIT`. Missing
`PIPE_FAIL` on a script with a meaningful pipeline → still its own
finding.

## Practical review notes

- Confirm the shebang is actually `#!/usr/bin/env zsh` or `#!/bin/zsh`
  when Zsh-only syntax (native array indexing, `(#i)` glob qualifiers,
  `zsh/` modules, `${(f)"$(cmd)"}` parameter-expansion flags) is in use —
  a script using Zsh-only syntax under a `sh`/`bash` shebang will fail
  outright on systems where Zsh isn't the resolved interpreter.
- `${(f)"$(cmd)"}`-style parameter-expansion flags are powerful
  Zsh-specific syntax (split on newlines, join arrays, etc.) — verify they
  aren't hiding an unquoted expansion bug underneath the flag syntax.
- All the security guidance in `security-checklist.md` (eval, temp files,
  credentials, least privilege, `PATH`) applies identically to Zsh — none
  of Zsh's word-splitting differences change those risks.
