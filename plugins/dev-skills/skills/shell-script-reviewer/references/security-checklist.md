# Shell Script Security Checklist

Security findings are always **highest priority** — surface them before
correctness, portability, or style findings (see `SKILL.md` step 3).

## Injection prevention

- **Never pass untrusted/external input to `eval`.** `eval` re-parses its
  argument as shell code, so any user-controlled substring becomes
  arbitrary command execution. If dynamic dispatch is genuinely needed,
  use an allowlisted `case` statement mapping known values to actions, or
  an associative array of function references — never string-build a
  command and `eval` it.

  ```bash
  # PROBLEM: attacker-controlled $action becomes arbitrary code
  eval "$action"

  # FIX: allowlist
  case "$action" in
    start|stop|restart) "$action" ;;
    *) echo "unknown action: $action" >&2; exit 1 ;;
  esac
  ```

- **Quote every expansion of external input** — command-line args,
  environment variables, file contents, command output. Unquoted
  expansion is a word-splitting/globbing bug (see `bash-pitfalls.md`,
  ShellCheck SC2086/SC2046) and, when the split pieces are then passed to
  another command, it can become an argument-injection vector (e.g.
  smuggling in an unexpected `-e` or `--option` flag).

- **Validate all external input against an allowlist, not a blocklist.**
  Blocklisting specific bad characters/strings is always incomplete;
  define what a valid value looks like (regex, fixed set of values, numeric
  range) and reject anything else.

- **Don't build SQL, `awk`/`sed` programs, or other embedded-language code
  by interpolating shell variables into a string that's then executed.**
  Same injection class as `eval`. Pass values as separate arguments/bind
  parameters where the underlying tool supports it.

## Temp files

- **Never use a predictable temp file path** (`/tmp/myscript.$$`,
  `/tmp/myscript.tmp`). Predictable names in a world-writable directory
  are vulnerable to symlink races and pre-creation attacks (TOCTOU).
- **Always use `mktemp`/`mktemp -d`** to create temp files/dirs with a
  random, race-free name and safe permissions.
- **Always clean up with `trap ... EXIT`** so the temp file/dir is removed
  even on error paths or early exit — don't rely on cleanup at the bottom
  of the script running.

  ```bash
  tmpdir=$(mktemp -d) || exit 1
  trap 'rm -rf -- "$tmpdir"' EXIT
  ```

## Credentials

- **Never pass secrets as command-line arguments.** Arguments are visible
  to any other user on the system via `ps`, `/proc/<pid>/cmdline`, and
  process accounting logs. Use environment variables (still visible to the
  same UID, but not to other users via `ps`), a file with restricted
  permissions, or a secrets-manager integration instead.
- **Restrict permissions on any file containing credentials**
  (`chmod 600`), and set a restrictive `umask` before creating it rather
  than creating it world-readable and fixing permissions after (a race
  window exists between creation and the `chmod`).
- **Don't echo/log secrets**, including via `set -x`/`bash -x` tracing
  left enabled in production, or via error messages that include the full
  command line.

## Least privilege

- **Drop elevated privileges immediately after the operation that needs
  them**, rather than running the rest of the script as root/setuid.
- **Avoid unnecessary root/SUID execution.** If only one operation in the
  script needs elevation, isolate it (e.g., a single `sudo` call for that
  line) rather than requiring the whole script to run privileged.
- Flag any script that requires root but doesn't clearly document why, or
  that could be rewritten to need elevation for only part of its work.

## `PATH` and executable resolution

- **Sanitize `PATH` explicitly** in scripts run via cron, setuid contexts,
  or on behalf of other users — don't inherit an untrusted caller's
  `PATH`. Set it explicitly near the top:
  `PATH=/usr/local/bin:/usr/bin:/bin`.
- **Use absolute paths for security-sensitive commands** (anything
  involved in privilege changes, file permission changes, or destructive
  operations) rather than relying on `PATH` lookup, so a malicious
  earlier-in-PATH binary can't be substituted.

## Additional checks

- **Race conditions (TOCTOU)** — a `[ -e file ]` check followed by a
  separate operation on `file` has a window where the file can change
  underneath the script. Prefer atomic operations (`mkdir` for lockfiles,
  `mktemp` for temp files, `ln` for atomic rename-based locks) over
  check-then-act.
- **Command injection via filenames** — a filename beginning with `-` can
  be interpreted as a flag by the next command it's passed to (ShellCheck
  SC2035). Use `--` or `./` prefixes.
- **World-writable directories** — creating any file in `/tmp` or another
  shared directory without `mktemp`'s race-free creation is a symlink
  attack vector.
- **Don't trust `$0`, `argv[0]`, or the script's own path** for anything
  security-sensitive; these are attacker-influenced when the script is
  invoked in unusual ways.
