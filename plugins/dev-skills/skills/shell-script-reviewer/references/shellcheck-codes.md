# ShellCheck Code Reference

Curated from https://www.shellcheck.net/wiki/ — treat these as ground truth
for Bash/POSIX findings. When ShellCheck reports one of these, cite the
code, explain the risk in the script's actual context, and show the fix;
don't re-derive the rule from scratch.

| Code | Title | Cause | Risk | Fix |
|------|-------|-------|------|-----|
| SC2086 | Double quote to prevent globbing and word splitting | Unquoted expansion of `$var`, `$1`, `$*`/`$@` | Word-splitting on IFS and glob expansion break the script on inputs with spaces or glob characters | Quote it: `"$var"`, `"$1"`, `"$@"` |
| SC2046 | Quote this to prevent word splitting | Unquoted command substitution `$(cmd)` | Same word-splitting/globbing risk, applied to command output | Quote it: `"$(cmd)"`; if splitting is genuinely wanted, use `read -a` or `mapfile` instead |
| SC2155 | Declare and assign separately to avoid masking return values | `local`/`export`/`readonly` combined with `var=$(cmd)` in one statement | The declaration's exit status overwrites the command substitution's exit status, so `$?` and `set -e` see the wrong result | Split into two statements: `local var; var=$(cmd)` |
| SC2164 | Use `cd ... \|\| exit` in case `cd` fails | `cd` called with no failure handling | A failed `cd` (bad path, permissions, broken symlink) leaves later commands running in the wrong directory — dangerous before `rm`/writes | `cd dir \|\| exit`, `cd dir \|\| return`, or wrap in `if cd dir; then ... fi` |
| SC2045 | Iterating over `ls` output is fragile. Use globs | `for f in $(ls *.ext)` | Breaks on filenames with spaces, newlines, or glob characters; `ls` output was never meant to be parsed | `for f in *.ext; do [[ -e "$f" ]] \|\| break; ...; done` |
| SC3045 | Shell built-in flags unsupported in POSIX sh | bash-only flags (`read -e`, `export -f`, `wait -n`, etc.) used under a `sh`/`dash` shebang | Script fails or behaves differently when actually run under `dash`/POSIX sh despite the shebang claiming portability | Change the shebang to `#!/bin/bash` if the feature is needed, or rewrite with a POSIX-portable equivalent |
| SC1036 | `(` is invalid here | Literal `(` used where shell syntax expects a word, often `(cmd)` instead of `$(cmd)`, or non-shell syntax copied in | Script fails to parse | Use `$(cmd)` for substitution, quote literal parens, or remove them for a plain function call |
| SC2181 | Check exit code directly, not indirectly with `$?` | `cmd; if [ $? -eq 0 ]; then` | An intervening command (even `echo`) silently changes what `$?` refers to; also fights `set -e` | `if cmd; then` / `if ! cmd; then`; for captured output: `if ! out=$(cmd); then` |
| SC2068 | Double quote array expansions | Unquoted `$@` or `${array[@]}` | Each element is re-split and glob-expanded instead of passed through as one argument each | Quote it: `"$@"`, `"${array[@]}"` |
| SC2034 | Variable appears unused | A variable is assigned but never read | Usually a typo (wrong name referenced elsewhere) or genuinely dead code | Fix the typo, use `_` for intentional throwaways, or `# shellcheck disable=SC2034` with a reason if it's read indirectly |
| SC2115 | Use `"${var:?}"` to ensure this never expands to `/*` | Unguarded variable in a destructive path, e.g. `rm -rf "$root/"*` | If `$root` is empty or unset, the command becomes `rm -rf /*` — catastrophic data loss | `rm -rf "${root:?}/"*` so an empty/unset variable aborts instead of expanding |
| SC2006 | Use `$(...)` instead of backticks | Legacy `` `cmd` `` substitution | Backtick quoting/escaping rules are inconsistent and nesting is painful and error-prone | Replace with `$(cmd)` |
| SC2001 | Consider parameter expansion instead of `sed` | `echo "$var" \| sed 's/x/y/'` for a simple substitution | Unnecessary subprocess and pipe for something Bash can do natively | `"${var/x/y}"` (first match) or `"${var//x/y}"` (all matches) |
| SC2148 | Add a shebang | No `#!` line, so ShellCheck (and the OS) can't determine the target shell | Shell-specific advice can't be given, and direct execution depends on the caller's default shell | Add `#!/bin/bash` (or the correct interpreter) as line 1 |
| SC2035 | Use `./*glob*` or `-- *glob*` | Bare glob like `rm *` where a matched filename can start with `-` | A file named e.g. `-f` gets parsed as a flag instead of a filename | `rm ./*` or `rm -- *` |
| SC2076 | Don't quote the right-hand side of `=~` | `[[ $x =~ "^foo$" ]]` | Quoting forces literal string matching instead of regex — the pattern silently stops being a regex | Drop the quotes and escape special characters instead: `[[ $x =~ ^foo$ ]]` |
| SC2016 | Expressions don't expand in single quotes | `$var` or `$(cmd)` written inside `'...'` | Output is the literal text `$var`, not its value — usually not what was intended | Use double quotes if expansion is wanted; keep single quotes (and silence the warning) if the literal text is intentional |
| SC1091 | Not following sourced file | ShellCheck can't resolve a `source`/`.` target (missing file, not passed on the command line, dynamic path) | Bugs inside the sourced file go unchecked | Add `# shellcheck source=path/to/file`, or `source=/dev/null` if it's genuinely unavailable to the checker |
| SC2166 | Avoid `-a`/`-o` in `[ ]` | `[ p -a q ]` / `[ p -o q ]` | POSIX deprecated these — behavior is ambiguous once arguments contain `-` or `!` | `[ p ] && [ q ]` / `[ p ] || [ q ]` |
| SC2129 | Consolidate multiple redirects | Repeated `cmd >> file` for a sequence of commands | Reopens the file for every command; unnecessarily slow and can interact oddly with traps | Group the commands and redirect once: `{ cmd1; cmd2; cmd3; } >> file` |
| SC2059 | Don't put variables in printf's format string | `printf "$msg\n"` | If `$msg` contains `%` or backslash escapes, printf misinterprets them | `printf '%s\n' "$msg"` |
| SC2236 | Use `-n`/`-z` directly instead of `! -z`/`! -n` | `[ ! -z "$x" ]` | Purely a readability nit — double negative | `[ -n "$x" ]` |

## Limits of ShellCheck coverage

ShellCheck is static analysis: it cannot evaluate runtime-constructed
values. Do not treat "ShellCheck found nothing" as "this construct is
safe" for:

- `eval` with any input that isn't a fixed string literal.
- Dynamic command names built from variables (`"$cmd" "$@"` where `$cmd` is
  user-influenced).
- Indirect variable references / `${!name}`.
- Anything inside a string handed to a different interpreter (e.g. an
  embedded `awk`/`sed`/`sql` snippet with interpolated shell variables).

Flag these as "needs runtime verification" per the escalation rule in
`SKILL.md`, and recommend a BATS test rather than asserting safety.

ShellCheck also has **no native Zsh dialect**. Running it with
`--shell=bash` against a Zsh script is a best-effort approximation for
portable constructs only — see `zsh-guide.md` for what it will miss.
ShellCheck **does not support Fish at all** — see `fish-guide.md`.
