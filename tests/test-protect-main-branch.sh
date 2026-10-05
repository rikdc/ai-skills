#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOOK_SCRIPT="$PROJECT_ROOT/plugins/security-hooks/hooks/protect-main-branch.sh"

TESTS_PASSED=0
TESTS_FAILED=0

test_passed() {
    echo "✓ $1"
    ((TESTS_PASSED++)) || true
}

test_failed() {
    echo "✗ $1"
    ((TESTS_FAILED++)) || true
}

run_test() {
    local test_name="$1"
    local branch="$2"
    local tool="$3"
    local should_block="$4"

    local tmpfile
    tmpfile=$(mktemp)

    cat > "$tmpfile" <<EOF
{
  "hook_event_name": "PreToolUse",
  "tool_name": "$tool",
  "tool_input": {}
}
EOF

    local output
    local exit_code=0

    if output=$(TEST_BRANCH_NAME="$branch" CLAUDE_PROJECT_DIR="$PROJECT_ROOT" \
        bash "$HOOK_SCRIPT" < "$tmpfile" 2>&1); then
        exit_code=0
    else
        exit_code=$?
    fi

    rm -f "$tmpfile"

    # PreToolUse always exits 0; the decision lives in the JSON on stdout.
    if [[ $exit_code -ne 0 ]]; then
        test_failed "$test_name (hook exited $exit_code, expected 0)"
        return
    fi

    local decision=""
    if [[ -n "$output" ]]; then
        decision=$(echo "$output" | jq -r '.hookSpecificOutput.permissionDecision // empty' 2>/dev/null || echo "")
    fi

    if [[ "$should_block" == "true" ]]; then
        if [[ "$decision" == "deny" ]]; then
            test_passed "$test_name"
        else
            test_failed "$test_name (expected deny, got '${decision:-no decision}')"
        fi
    else
        if [[ -z "$decision" ]]; then
            test_passed "$test_name"
        else
            test_failed "$test_name (expected allow, got '$decision')"
        fi
    fi
}

echo "Running Protected Branch Hook Tests..."
echo "========================================"

run_bash_test() {
    local test_name="$1"
    local branch="$2"
    local command="$3"
    local should_block="$4"

    local tmpfile
    tmpfile=$(mktemp)

    jq -n --arg cmd "$command" '{
      hook_event_name: "PreToolUse",
      tool_name: "Bash",
      tool_input: { command: $cmd }
    }' > "$tmpfile"

    local output
    local exit_code=0

    if output=$(TEST_BRANCH_NAME="$branch" CLAUDE_PROJECT_DIR="$PROJECT_ROOT" \
        bash "$HOOK_SCRIPT" < "$tmpfile" 2>&1); then
        exit_code=0
    else
        exit_code=$?
    fi

    rm -f "$tmpfile"

    if [[ $exit_code -ne 0 ]]; then
        test_failed "$test_name (hook exited $exit_code, expected 0)"
        return
    fi

    local decision=""
    if [[ -n "$output" ]]; then
        decision=$(echo "$output" | jq -r '.hookSpecificOutput.permissionDecision // empty' 2>/dev/null || echo "")
    fi

    if [[ "$should_block" == "true" ]]; then
        if [[ "$decision" == "deny" ]]; then
            test_passed "$test_name"
        else
            test_failed "$test_name (expected deny, got '${decision:-no decision}')"
        fi
    else
        if [[ -z "$decision" ]]; then
            test_passed "$test_name"
        else
            test_failed "$test_name (expected allow, got '$decision')"
        fi
    fi
}

run_test "Block Edit on main branch" "main" "Edit" "true"
run_test "Block Write on main branch" "main" "Write" "true"
run_test "Block Task on main branch" "main" "Task" "true"

run_test "Block Edit on master branch" "master" "Edit" "true"
run_test "Block Edit on production branch" "production" "Edit" "true"
run_test "Block Edit on release branch" "release" "Edit" "true"

run_test "Allow Edit on feature branch" "feature/test" "Edit" "false"
run_test "Allow Write on feature branch" "alice/PROJ-123-feature" "Write" "false"
run_test "Allow Bash on feature branch" "bob/fix-bug" "Bash" "false"
run_test "Allow Task on feature branch" "carol/test-workflow" "Task" "false"

run_test "Allow Read on main branch" "main" "Read" "false"
run_test "Allow Grep on main branch" "main" "Grep" "false"

echo ""
echo "--- Regression: read-only and non-git Bash on main (was wrongly blocked) ---"

# Every one of these was denied by the pre-fix hook, which never read
# tool_input.command and blocked all Bash on a protected branch outright.
run_bash_test "Allow openssl s_client cert inspection" "main" \
    "echo | openssl s_client -connect h:443 -servername h | openssl x509 -noout -issuer" "false"
run_bash_test "Allow curl piped into grep" "main" \
    "curl -sSv https://h/p 2>&1 | grep -iE 'issuer:|subject:'" "false"
run_bash_test "Allow git status --short" "main" "git status --short" "false"
run_bash_test "Allow git diff on a path" "main" "git diff path/to/file" "false"
run_bash_test "Allow sudo sh -c tar piped to ssh" "main" \
    "sudo sh -c 'tar c -C /d a b | ssh -i key user@host'" "false"

run_bash_test "Allow curl piped into head" "main" "curl -sSI https://h/p | head -1" "false"
run_bash_test "Allow git add -N / reset -- / diff --cached" "main" \
    "git add -N file ; git reset -- file ; git diff --cached --name-only" "false"

echo ""
echo "--- Regression: read-only git and lookalike strings ---"

run_bash_test "Allow git log" "main" "git log --oneline -5" "false"
run_bash_test "Allow git show" "main" "git show HEAD --stat" "false"
run_bash_test "Allow git blame" "main" "git blame README.md" "false"
run_bash_test "Allow git fetch" "main" "git fetch origin" "false"
run_bash_test "Allow git branch listing" "main" "git branch -a" "false"
run_bash_test "Allow plain find" "main" "find . -name '*.sh' -not -path './.git/*'" "false"
# Dangerous-looking tokens that are only ever data, never a git write.
run_bash_test "Allow grep for the word push" "main" "grep -rn 'push' plugins/" "false"
run_bash_test "Allow echo mentioning git commit" "main" "echo 'run git commit later'" "false"
run_bash_test "Allow URL containing main and push" "main" \
    "curl -s https://example.com/main/push | wc -l" "false"
run_bash_test "Allow reading a file under .git" "main" "cat .git/HEAD" "false"

echo ""
echo "--- Genuine protected-branch writes must still be blocked ---"

run_bash_test "Block git commit" "main" "git commit -m 'change'" "true"
run_bash_test "Block git commit -am" "main" "git commit -am 'change'" "true"
run_bash_test "Block git push" "main" "git push" "true"
run_bash_test "Block git push origin main" "main" "git push origin main" "true"
run_bash_test "Block git merge" "main" "git merge feature/x" "true"
run_bash_test "Block git rebase" "main" "git rebase origin/main" "true"
run_bash_test "Block git cherry-pick" "main" "git cherry-pick abc123" "true"
run_bash_test "Block git revert" "main" "git revert HEAD" "true"
run_bash_test "Block git reset --hard" "main" "git reset --hard HEAD~1" "true"
run_bash_test "Block git reset to a commit-ish" "main" "git reset HEAD~1" "true"
run_bash_test "Block git branch -D" "main" "git branch -D old-feature" "true"
run_bash_test "Block git branch -f" "main" "git branch -f main abc123" "true"
run_bash_test "Block git checkout -B" "main" "git checkout -B main origin/main" "true"
run_bash_test "Block git switch to another branch" "main" "git switch other" "true"
run_bash_test "Block git update-ref" "main" "git update-ref refs/heads/main abc123" "true"

echo ""
echo "--- Writes hidden behind wrappers, pipelines and .git/ ---"

run_bash_test "Block git commit behind sudo" "main" "sudo git commit -m x" "true"
run_bash_test "Block git push behind env assignment" "main" "GIT_AUTHOR_NAME=x git push" "true"
run_bash_test "Block git push wrapped in sh -c" "main" "sh -c 'git push origin main'" "true"
run_bash_test "Block git commit later in a pipeline" "main" "echo hi && git commit -m x" "true"
run_bash_test "Block git commit with -C global option" "main" "git -C /repo commit -m x" "true"
run_bash_test "Block rm under .git/" "main" "rm -f .git/refs/heads/main" "true"
run_bash_test "Block redirection into .git/" "main" "echo abc123 > .git/refs/heads/main" "true"

echo ""
echo "--- Feature branches are unaffected regardless of command ---"

run_bash_test "Allow git push on feature branch" "feature/test" "git push origin feature/test" "false"
run_bash_test "Allow git commit on feature branch" "alice/PROJ-1" "git commit -m 'work'" "false"

echo ""
echo "--- Creating a feature branch is the remedy and must be allowed ---"

# The deny message instructs the user to run exactly this; blocking it would
# trap them on the protected branch with no sanctioned way off.
run_bash_test "Allow git checkout -b" "main" "git checkout -b alice/new-feature" "false"
run_bash_test "Allow git switch -c" "main" "git switch -c alice/new-feature" "false"

# The deny payload must be parseable JSON: the previous heredoc embedded raw
# newlines in a string literal, which no JSON parser accepts.
deny_output=$(echo '{"tool_name":"Edit","tool_input":{}}' |
    TEST_BRANCH_NAME="main" CLAUDE_PROJECT_DIR="$PROJECT_ROOT" bash "$HOOK_SCRIPT")
if echo "$deny_output" | jq -e '.hookSpecificOutput.permissionDecisionReason' >/dev/null 2>&1; then
    test_passed "Deny payload is valid JSON"
else
    test_failed "Deny payload is valid JSON"
fi

override_output=$(echo '{"tool_name":"Edit","tool_input":{}}' |
    TEST_BRANCH_NAME="main" CLAUDE_PROJECT_DIR="$PROJECT_ROOT" \
    ALLOW_PROTECTED_BRANCH_EDIT=1 bash "$HOOK_SCRIPT")
if [[ -z "$override_output" ]]; then
    test_passed "ALLOW_PROTECTED_BRANCH_EDIT bypasses protection"
else
    test_failed "ALLOW_PROTECTED_BRANCH_EDIT bypasses protection"
fi

bash_override_output=$(jq -n '{tool_name:"Bash",tool_input:{command:"git push origin main"}}' |
    TEST_BRANCH_NAME="main" CLAUDE_PROJECT_DIR="$PROJECT_ROOT" \
    ALLOW_PROTECTED_BRANCH_EDIT=1 bash "$HOOK_SCRIPT")
if [[ -z "$bash_override_output" ]]; then
    test_passed "ALLOW_PROTECTED_BRANCH_EDIT bypasses protection for Bash writes"
else
    test_failed "ALLOW_PROTECTED_BRANCH_EDIT bypasses protection for Bash writes"
fi

echo ""
echo "--- Real repositories: the branch is read where the write lands ---"

# A main checkout plus a linked worktree on a feature branch. No
# TEST_BRANCH_NAME here: the hook must resolve branches itself. It runs from /
# so that only the payload (.cwd, file_path, cd, git -C) can point it anywhere.
FIXTURE="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$FIXTURE"' EXIT
git_quiet() { git -c user.name=test -c user.email=test@example.com -c commit.gpgsign=false -c init.defaultBranch=main "$@" >/dev/null 2>&1; }
git_quiet init "$FIXTURE/repo"
git_quiet -C "$FIXTURE/repo" commit --allow-empty -m init
git_quiet -C "$FIXTURE/repo" worktree add -b feature/wt "$FIXTURE/wt"
mkdir -p "$FIXTURE/plain"
MAIN_REPO="$FIXTURE/repo"
WT="$FIXTURE/wt"

check_real() {
    local test_name="$1" payload="$2" should_block="$3"
    local output decision
    if ! output=$(echo "$payload" | (cd / && env -u TEST_BRANCH_NAME \
        CLAUDE_PROJECT_DIR="$PROJECT_ROOT" bash "$HOOK_SCRIPT") 2>&1); then
        test_failed "$test_name (hook exited non-zero)"
        return
    fi
    decision=$(echo "$output" | jq -r '.hookSpecificOutput.permissionDecision // empty' 2>/dev/null || echo "")
    if [[ "$should_block" == "true" && "$decision" == "deny" ]] ||
        [[ "$should_block" == "false" && -z "$decision" ]]; then
        test_passed "$test_name"
    else
        test_failed "$test_name (expected block=$should_block, got '${decision:-allow}')"
    fi
}

run_file_test() {
    local test_name="$1" cwd="$2" tool="$3" path="$4" should_block="$5"
    check_real "$test_name" "$(jq -n --arg t "$tool" --arg c "$cwd" --arg p "$path" \
        '{tool_name:$t, cwd:$c, tool_input:{file_path:$p}}')" "$should_block"
}

run_cmd_test() {
    local test_name="$1" cwd="$2" command="$3" should_block="$4"
    check_real "$test_name" "$(jq -n --arg c "$cwd" --arg cmd "$command" \
        '{tool_name:"Bash", cwd:$c, tool_input:{command:$cmd}}')" "$should_block"
}

# Edit / Write / Task
run_file_test "Allow Edit in worktree from a session on main" "$MAIN_REPO" Edit "$WT/a.txt" "false"
run_file_test "Allow Write creating new dirs in worktree" "$MAIN_REPO" Write "$WT/new/dir/b.txt" "false"
run_file_test "Allow relative Write into worktree" "$MAIN_REPO" Write "../wt/c.txt" "false"
run_file_test "Block Edit in main checkout from a worktree session" "$WT" Edit "$MAIN_REPO/a.txt" "true"
run_file_test "Block Write in main checkout from its own session" "$MAIN_REPO" Write "$MAIN_REPO/a.txt" "true"
run_file_test "Allow Write outside any repository" "$MAIN_REPO" Write "$FIXTURE/plain/d.txt" "false"
check_real "Block Task from a session on main" \
    "$(jq -n --arg c "$MAIN_REPO" '{tool_name:"Task", cwd:$c, tool_input:{}}')" "true"
check_real "Allow Task from a worktree session" \
    "$(jq -n --arg c "$WT" '{tool_name:"Task", cwd:$c, tool_input:{}}')" "false"

# Bash: cd and git -C move the directory a write is judged in
run_cmd_test "Allow cd worktree && git commit from main session" "$MAIN_REPO" "cd $WT && git commit -m x" "false"
run_cmd_test "Allow relative cd into worktree" "$MAIN_REPO" "cd ../wt && git commit -m x" "false"
run_cmd_test "Allow git -C worktree commit from main session" "$MAIN_REPO" "git -C $WT commit -m x" "false"
run_cmd_test "Allow sh -c cd worktree && git push" "$MAIN_REPO" "sh -c 'cd $WT && git push'" "false"
run_cmd_test "Allow git commit in a worktree session" "$WT" "git commit -m x" "false"
run_cmd_test "Block git commit in a main session" "$MAIN_REPO" "git commit -m x" "true"
run_cmd_test "Block cd main && git push from worktree session" "$WT" "cd $MAIN_REPO && git push" "true"
run_cmd_test "Block git -C main commit from worktree session" "$WT" "git -C $MAIN_REPO commit -m x" "true"
run_cmd_test "Block second cd back to main" "$MAIN_REPO" "cd $WT && git commit -m x; cd $MAIN_REPO && git push" "true"
run_cmd_test "Block rm under main .git from worktree session" "$WT" "cd $MAIN_REPO && rm -f .git/refs/heads/main" "true"
# Unresolvable targets keep the previous directory rather than failing open.
# shellcheck disable=SC2016 # the literal $SOMEWHERE is the point
run_cmd_test "Block cd to a variable then commit on main" "$MAIN_REPO" 'cd "$SOMEWHERE" && git commit -m x' "true"
run_cmd_test "Block cd to a missing dir then commit on main" "$MAIN_REPO" "cd $FIXTURE/nope && git commit -m x" "true"
run_cmd_test "Allow git commit outside any repository" "$FIXTURE/plain" "git commit -m x" "false"

echo ""
echo "========================================"
echo "Tests passed: $TESTS_PASSED"
echo "Tests failed: $TESTS_FAILED"

if [[ $TESTS_FAILED -gt 0 ]]; then
    exit 1
fi

exit 0
