#!/usr/bin/env bash

set -euo pipefail

LOG_DIR="${CLAUDE_PROJECT_DIR:-$(pwd)}/.claude"
LOG_FILE="$LOG_DIR/protect-main-branch.log"

log() {
    local level="$1"
    shift
    [[ -d "$LOG_DIR" ]] || return 0
    echo "[$(date -u +"%Y-%m-%dT%H:%M:%SZ")] $level: $*" >> "$LOG_FILE" 2>/dev/null || true
}

log_debug() { log DEBUG "$@"; }
log_info()  { log INFO "$@"; }
log_warn()  { log WARN "$@"; }
log_error() { log ERROR "$@"; }

# PreToolUse hooks always exit 0. A deny is expressed through
# hookSpecificOutput.permissionDecision on stdout, not an exit code.
allow() {
    exit 0
}

deny() {
    local reason="$1"
    jq -n --arg reason "$reason" '{
      hookSpecificOutput: {
        hookEventName: "PreToolUse",
        permissionDecision: "deny",
        permissionDecisionReason: $reason
      }
    }'
    exit 0
}

deny_branch_write() {
    local branch="$1"
    local detail="$2"
    log_error "PROTECTED BRANCH VIOLATION: $detail on protected branch '$branch'"
    deny "Blocked on protected branch '$branch': $detail

Protected branches enforce PR-based workflows to ensure:
- Code review and quality standards
- CI/CD pipeline validation
- Collaboration and knowledge sharing
- Audit trail for all changes

To proceed:
1. Create a feature branch (this is allowed from a protected branch):
   git checkout -b your-name/feature-description

2. Make your changes on the feature branch

3. Push and create a Pull Request:
   git push -u origin your-name/feature-description

Protected branches: ${PROTECTED_BRANCHES[*]}

Read-only commands are not affected: git status, git diff, git log, git show,
and every non-git command run through without inspection.

For emergency changes, set ALLOW_PROTECTED_BRANCH_EDIT=1 in the environment
Claude Code itself is launched with (the hook reads its own environment, so an
inline VAR=1 prefix on the command will not reach it).

This hook ensures repository safety and collaboration best practices."
}

# ---------------------------------------------------------------------------
# Shell parsing helpers
# ---------------------------------------------------------------------------

# Split a command string into pipeline/list segments on unquoted | ; & and
# newlines. Quotes and backslash escapes are respected so that separators
# inside 'single quotes' or "double quotes" do not split the segment.
SEGMENTS=()
split_segments() {
    local input="$1"
    SEGMENTS=()

    local current="" quote="" i char
    for ((i = 0; i < ${#input}; i++)); do
        char="${input:i:1}"

        if [[ -n "$quote" ]]; then
            current+="$char"
            # Backslash escapes are only special inside double quotes.
            if [[ "$char" == "\\" && "$quote" == '"' && $((i + 1)) -lt ${#input} ]]; then
                ((i++))
                current+="${input:i:1}"
            elif [[ "$char" == "$quote" ]]; then
                quote=""
            fi
            continue
        fi

        case "$char" in
            "'"|'"')
                quote="$char"
                current+="$char"
                ;;
            "\\")
                current+="$char"
                if [[ $((i + 1)) -lt ${#input} ]]; then
                    ((i++))
                    current+="${input:i:1}"
                fi
                ;;
            "|"|";"|"&"|$'\n')
                SEGMENTS+=("$current")
                current=""
                ;;
            *)
                current+="$char"
                ;;
        esac
    done

    SEGMENTS+=("$current")
}

# Tokenize one segment into argv, stripping one level of quoting.
TOKENS=()
tokenize() {
    local input="$1"
    TOKENS=()

    local current="" quote="" started=0 i char
    for ((i = 0; i < ${#input}; i++)); do
        char="${input:i:1}"

        if [[ -n "$quote" ]]; then
            if [[ "$char" == "\\" && "$quote" == '"' && $((i + 1)) -lt ${#input} ]]; then
                ((i++))
                current+="${input:i:1}"
            elif [[ "$char" == "$quote" ]]; then
                quote=""
            else
                current+="$char"
            fi
            continue
        fi

        case "$char" in
            "'"|'"')
                quote="$char"
                started=1
                ;;
            "\\")
                if [[ $((i + 1)) -lt ${#input} ]]; then
                    ((i++))
                    current+="${input:i:1}"
                    started=1
                fi
                ;;
            " "|$'\t'|$'\r')
                if [[ $started -eq 1 ]]; then
                    TOKENS+=("$current")
                    current=""
                    started=0
                fi
                ;;
            *)
                current+="$char"
                started=1
                ;;
        esac
    done

    if [[ $started -eq 1 ]]; then
        TOKENS+=("$current")
    fi
}

# Drop leading VAR=value assignments and transparent wrappers (sudo, env, nice,
# time, ...) so that TOKENS[0] is the command that actually runs. Wrapper flags
# that take a value are consumed alongside the wrapper.
strip_wrappers() {
    local changed=1
    while [[ $changed -eq 1 && ${#TOKENS[@]} -gt 0 ]]; do
        changed=0

        # Leading environment assignments: FOO=bar git push
        while [[ ${#TOKENS[@]} -gt 0 && "${TOKENS[0]}" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; do
            TOKENS=("${TOKENS[@]:1}")
            changed=1
        done

        [[ ${#TOKENS[@]} -gt 0 ]] || break

        case "$(basename -- "${TOKENS[0]}")" in
            sudo|doas)
                TOKENS=("${TOKENS[@]:1}")
                changed=1
                # Consume sudo flags; -u/-g/-C/-p take a separate argument.
                while [[ ${#TOKENS[@]} -gt 0 && "${TOKENS[0]}" == -* ]]; do
                    case "${TOKENS[0]}" in
                        -u|-g|-C|-p|-h|-r|-t|--user|--group|--prompt)
                            TOKENS=("${TOKENS[@]:2}")
                            ;;
                        *)
                            TOKENS=("${TOKENS[@]:1}")
                            ;;
                    esac
                done
                ;;
            env)
                TOKENS=("${TOKENS[@]:1}")
                changed=1
                while [[ ${#TOKENS[@]} -gt 0 && "${TOKENS[0]}" == -* ]]; do
                    TOKENS=("${TOKENS[@]:1}")
                done
                ;;
            nice|ionice|nohup|stdbuf|time|command|builtin|exec)
                TOKENS=("${TOKENS[@]:1}")
                changed=1
                while [[ ${#TOKENS[@]} -gt 0 && "${TOKENS[0]}" == -* ]]; do
                    TOKENS=("${TOKENS[@]:1}")
                done
                ;;
            *)
                # Not a wrapper: TOKENS[0] is the real command.
                ;;
        esac
    done
}

# ---------------------------------------------------------------------------
# git policy
# ---------------------------------------------------------------------------

# git subcommands that always write to the current branch or the remote.
git_subcommand_always_writes() {
    case "$1" in
        commit|push|merge|rebase|cherry-pick|revert|am|filter-branch|update-ref|fast-import)
            return 0
            ;;
        *)
            # Read-only or index-only subcommand: status, diff, log, add, ...
            ;;
    esac
    return 1
}

# Returns 0 (block) if this `git reset` invocation moves the branch pointer or
# destroys the working tree. `git reset -- path` and bare `git reset` only touch
# the index and are allowed.
git_reset_writes() {
    local -a args=("$@")
    local arg
    for arg in "${args[@]}"; do
        case "$arg" in
            --hard|--merge|--keep|--soft)
                return 0
                ;;
            --)
                # Path-limited unstage: git reset -- file
                return 1
                ;;
            *)
                ;;
        esac
    done
    # A bare commit-ish argument (git reset HEAD~1) moves the branch pointer.
    for arg in "${args[@]}"; do
        [[ "$arg" == -* ]] && continue
        return 0
    done
    return 1
}

# Returns 0 (block) for branch deletion, forced creation or renaming. Plain
# listing and ordinary branch creation are harmless.
git_branch_writes() {
    local arg
    for arg in "$@"; do
        case "$arg" in
            -D|-d|--delete|-f|--force|-M|-m|--move|-C|--copy|--edit-description)
                return 0
                ;;
            *)
                # Listing or plain creation: harmless.
                ;;
        esac
    done
    return 1
}

# Returns 0 (block) if checkout/switch moves HEAD off the protected branch or
# force-resets a branch ref.
#
# `git checkout -b` / `git switch -c` are deliberately ALLOWED: creating a
# feature branch is the remedy this hook's own deny message instructs the user
# to perform, so blocking it would trap them on the protected branch. The
# force variants (-B / -C) can reset an existing branch ref and stay blocked.
git_checkout_writes() {
    local arg
    for arg in "$@"; do
        case "$arg" in
            -B|--force-create)
                return 0
                ;;
            -b|-c|--create)
                return 1
                ;;
            --)
                # Path restore: git checkout -- file. Does not move HEAD.
                return 1
                ;;
            *)
                ;;
        esac
    done
    # A bare ref argument moves HEAD.
    for arg in "$@"; do
        [[ "$arg" == -* ]] && continue
        return 0
    done
    return 1
}

# ---------------------------------------------------------------------------
# Branch resolution
# ---------------------------------------------------------------------------
#
# The branch that matters is the one checked out where the write lands, not
# the one checked out in the hook's own working directory. A session started
# in a repository's main checkout routinely edits files in a linked worktree
# (or another repository) that sits on a feature branch.

# Branch checked out in the work tree containing $1. Empty when $1 is not in a
# git work tree or HEAD is detached.
branch_at() {
    local dir="$1"
    if [[ -n "${TEST_BRANCH_NAME:-}" ]]; then
        echo "$TEST_BRANCH_NAME"
        return 0
    fi
    git -C "$dir" branch --show-current 2>/dev/null || true
}

# Nearest existing directory at or above $1, so a Write that creates new
# directories is judged by the work tree it lands in.
existing_dir_for() {
    local path="$1"
    [[ -d "$path" ]] || path="$(dirname -- "$path")"
    while [[ ! -d "$path" && "$path" != "/" && "$path" != "." ]]; do
        path="$(dirname -- "$path")"
    done
    echo "$path"
}

# Resolve a directory argument (a cd target or a git -C value) against $1.
# Prints nothing when the target cannot be resolved statically - variables,
# command substitution, `cd -`, a path that does not exist - and callers then
# keep the directory they already had. That is the conservative choice: it is
# how the hook behaved before it tracked directories at all.
resolve_dir() {
    local base="$1" target="$2"
    # The quoted tildes are literal patterns matched against unexpanded input.
    # shellcheck disable=SC2088
    case "$target" in
        ''|'~') target="$HOME" ;;
        '~/'*) target="$HOME/${target:2}" ;;
        -) return 0 ;;
        *) ;;
    esac
    [[ "$target" == *'$'* || "$target" == *'`'* ]] && return 0
    [[ "$target" == /* ]] || target="$base/$target"
    [[ -d "$target" ]] || return 0
    (cd "$target" 2>/dev/null && pwd) || true
}

# Echo "<branch><TAB><reason>" and succeed when $1 is on a protected branch.
flag_if_protected() {
    local dir="$1" reason="$2" branch
    branch="$(branch_at "$dir")"
    if [[ -n "$branch" && "$branch" =~ $PROTECTED_PATTERN ]]; then
        printf '%s\t%s\n' "$branch" "$reason"
        return 0
    fi
    return 1
}

# Inspect a git invocation run from directory $1. TOKENS[0] is known to be git.
# Echoes "<branch><TAB><reason>" and returns 0 when the call must be blocked.
inspect_git() {
    local dir="$1"
    local -a args=("${@:3}")

    # Skip global options to find the subcommand. Options taking a value are
    # consumed with their argument; each -C moves the effective directory.
    local subcommand="" idx=0 resolved
    while [[ $idx -lt ${#args[@]} ]]; do
        case "${args[idx]}" in
            -C)
                resolved="$(resolve_dir "$dir" "${args[idx + 1]:-}")"
                [[ -z "$resolved" ]] || dir="$resolved"
                idx=$((idx + 2))
                ;;
            -c|--git-dir|--work-tree|--namespace|--exec-path)
                idx=$((idx + 2))
                ;;
            --git-dir=*|--work-tree=*|--namespace=*|--exec-path=*)
                idx=$((idx + 1))
                ;;
            -*)
                idx=$((idx + 1))
                ;;
            *)
                subcommand="${args[idx]}"
                break
                ;;
        esac
    done

    [[ -n "$subcommand" ]] || return 1

    local -a subargs=()
    if [[ $((idx + 1)) -lt ${#args[@]} ]]; then
        subargs=("${args[@]:idx+1}")
    fi

    local reason=""
    if git_subcommand_always_writes "$subcommand"; then
        reason="git $subcommand writes to the branch or remote"
    else
        case "$subcommand" in
            reset)
                if git_reset_writes "${subargs[@]+"${subargs[@]}"}"; then
                    reason="git reset moves the branch pointer or discards the working tree"
                fi
                ;;
            branch)
                if git_branch_writes "${subargs[@]+"${subargs[@]}"}"; then
                    reason="git branch deletes, renames or force-updates a branch ref"
                fi
                ;;
            checkout|switch)
                if git_checkout_writes "${subargs[@]+"${subargs[@]}"}"; then
                    reason="git $subcommand moves HEAD off the protected branch"
                fi
                ;;
            *)
                # Unrecognised subcommand: fail open, consistent with the parser.
                ;;
        esac
    fi

    [[ -n "$reason" ]] || return 1
    flag_if_protected "$dir" "$reason"
}

# Detect writes that bypass git entirely by editing files under .git/.
WRITE_COMMANDS="rm|mv|cp|tee|dd|truncate|ln|install|shred|sed|perl|chmod|chown"
inspect_git_dir_write() {
    local raw="$1"
    shift
    local -a args=("$@")

    local cmd
    cmd="$(basename -- "${args[0]:-}")"

    if [[ "$raw" =~ \>\>?[[:space:]]*[^[:space:]]*\.git/ ]]; then
        echo "shell redirection writes under .git/"
        return 0
    fi

    if [[ "$cmd" =~ ^($WRITE_COMMANDS)$ ]]; then
        local arg
        for arg in "${args[@]}"; do
            if [[ "$arg" == .git/* || "$arg" == */.git/* || "$arg" == .git || "$arg" == */.git ]]; then
                echo "$cmd writes under .git/"
                return 0
            fi
        done
    fi

    return 1
}

# Analyse one segment run from directory $3. Echoes "<branch><TAB><reason>"
# and returns 0 when it must block.
inspect_segment() {
    local segment="$1"
    local depth="${2:-0}"
    local dir="$3"

    tokenize "$segment"
    [[ ${#TOKENS[@]} -gt 0 ]] || return 1

    strip_wrappers
    [[ ${#TOKENS[@]} -gt 0 ]] || return 1

    local cmd
    cmd="$(basename -- "${TOKENS[0]}")"

    # Recurse one level into `sh -c '...'` so that a wrapped `git push` is still
    # caught, while `sh -c 'tar ... | ssh ...'` remains allowed on its merits.
    if [[ "$cmd" =~ ^(sh|bash|zsh|dash|ksh)$ && $depth -lt 2 ]]; then
        local i
        for ((i = 1; i < ${#TOKENS[@]}; i++)); do
            if [[ "${TOKENS[i]}" == "-c" && $((i + 1)) -lt ${#TOKENS[@]} ]]; then
                local inner="${TOKENS[i + 1]}"
                local -a saved=("${TOKENS[@]}")
                local finding
                if finding="$(inspect_command "$inner" $((depth + 1)) "$dir")"; then
                    echo "$finding"
                    return 0
                fi
                TOKENS=("${saved[@]}")
                break
            fi
        done
    fi

    local reason
    if reason="$(inspect_git_dir_write "$segment" "${TOKENS[@]}")"; then
        flag_if_protected "$dir" "$reason" && return 0
    fi

    if [[ "$cmd" == "git" ]]; then
        inspect_git "$dir" "${TOKENS[@]}" && return 0
    fi

    # Every other command - openssl, curl, tar, ssh, make, read-only git - passes.
    return 1
}

# Analyse a full command line run from directory $3. A `cd` segment moves the
# directory that later segments are judged in, so `cd worktree && git commit`
# is checked against the worktree's branch.
inspect_command() {
    local command="$1"
    local depth="${2:-0}"
    local dir="$3"

    local -a segments
    split_segments "$command"
    segments=("${SEGMENTS[@]}")

    local segment finding resolved
    for segment in "${segments[@]}"; do
        [[ -n "${segment//[[:space:]]/}" ]] || continue

        tokenize "$segment"
        if [[ ${#TOKENS[@]} -gt 0 && "${TOKENS[0]}" =~ ^(cd|pushd)$ ]]; then
            local -a cd_args=("${TOKENS[@]:1}")
            # Drop -L / -P style flags; the remaining word is the target.
            while [[ ${#cd_args[@]} -gt 0 && "${cd_args[0]}" == -?* ]]; do
                cd_args=("${cd_args[@]:1}")
            done
            resolved="$(resolve_dir "$dir" "${cd_args[0]:-}")"
            [[ -z "$resolved" ]] || dir="$resolved"
            continue
        fi

        if finding="$(inspect_segment "$segment" "$depth" "$dir")"; then
            echo "$finding"
            return 0
        fi
    done

    return 1
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

log_debug "Hook script started"
log_debug "Working directory: $(pwd)"

STDIN_CONTENT=$(cat)

TOOL_NAME=$(echo "$STDIN_CONTENT" | jq -r '.tool_name // empty')
log_debug "Tool name: $TOOL_NAME"

if [[ -z "$TOOL_NAME" ]]; then
    log_error "No tool name found in stdin"
    allow
fi

PROTECTED_TOOLS_PATTERN="^(Edit|Write|Bash|Task)$"
if [[ ! "$TOOL_NAME" =~ $PROTECTED_TOOLS_PATTERN ]]; then
    log_debug "Tool '$TOOL_NAME' is not a protected tool, skipping"
    allow
fi

if [[ -n "${ALLOW_PROTECTED_BRANCH_EDIT:-}" ]]; then
    log_warn "ALLOW_PROTECTED_BRANCH_EDIT set, bypassing protection for '$TOOL_NAME'"
    allow
fi

PROTECTED_BRANCHES=("main" "master" "production" "release")
PROTECTED_PATTERN="^(main|master|production|release)$"

# Claude Code passes the session's working directory as .cwd; fall back to our
# own when it is missing (older clients, manual invocation).
BASE_DIR=$(echo "$STDIN_CONTENT" | jq -r '.cwd // empty')
if [[ -z "$BASE_DIR" || ! -d "$BASE_DIR" ]]; then
    BASE_DIR="$(pwd)"
fi
log_debug "Base directory: $BASE_DIR"

# Bash is judged on what the command actually does, in the directory each
# write runs in.
if [[ "$TOOL_NAME" == "Bash" ]]; then
    COMMAND=$(echo "$STDIN_CONTENT" | jq -r '.tool_input.command // empty')

    if [[ -z "$COMMAND" ]]; then
        log_warn "Bash tool with no command field, allowing"
        allow
    fi

    log_debug "Inspecting command: $COMMAND"

    # Fail open: anything the parser cannot read is allowed, not denied.
    if ! FINDING="$(inspect_command "$COMMAND" 0 "$BASE_DIR" 2>/dev/null)"; then
        log_info "Command writes nothing on a protected branch, allowing: $COMMAND"
        allow
    fi

    deny_branch_write "${FINDING%%$'\t'*}" "${FINDING#*$'\t'}"
fi

# Edit and Write are judged by the work tree containing the target file; Task
# has no target path and is judged by the session directory.
TARGET_DIR="$BASE_DIR"
FILE_PATH=$(echo "$STDIN_CONTENT" | jq -r '.tool_input.file_path // empty')
if [[ -n "$FILE_PATH" ]]; then
    [[ "$FILE_PATH" == /* ]] || FILE_PATH="$BASE_DIR/$FILE_PATH"
    TARGET_DIR="$(existing_dir_for "$FILE_PATH")"
fi

CURRENT_BRANCH="$(branch_at "$TARGET_DIR")"
log_debug "Branch at $TARGET_DIR: $CURRENT_BRANCH"

if [[ -z "$CURRENT_BRANCH" ]]; then
    log_warn "Could not determine branch at $TARGET_DIR"
    allow
fi

if [[ ! "$CURRENT_BRANCH" =~ $PROTECTED_PATTERN ]]; then
    log_info "Branch '$CURRENT_BRANCH' is not protected, allowing tool execution"
    allow
fi

deny_branch_write "$CURRENT_BRANCH" "the $TOOL_NAME tool modifies files directly"
