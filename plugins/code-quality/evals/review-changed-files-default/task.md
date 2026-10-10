# Review the Pending Changes (Default Scope)

## Background

The **metrics** repository has a `feature/retry-backoff` branch — you are
currently on it — whose latest commit is waiting for review before it merges.
Reviewers invoke `/dev:review` from the code-quality plugin, which by default
reviews **only changed files**: modified/staged files, files changed in
recent commits, and the working branch versus main. Without `--all`, the
whole codebase is not in scope.

Start with `bash setup.sh` to create the repository state.

## Your Task

1. Determine the default review scope yourself (changed files per the rule
   above) using git status/diff/log — do not assume or review the whole
   repository.
2. Review the change for performance, security, maintainability, standards,
   and unhandled edge cases, and record concrete findings with code
   references.
3. Write the findings to `review.md` using the command's structured report
   format:
   - `📊 Change Summary` — commit hash, message, and scope overview
   - `🔍 Priority Findings` — High Priority, Medium Priority, Low Priority
   - `⚠️ Edge Cases & Risks`
   - `✅ Recommendations` — including any immediate action required before
     merge
4. Maintain a professional review tone; findings must reference actual code
   in the changed file — do not invent issues in files that are not in the
   default scope.

## Output Specification

- `review.md` in the working directory with the four sections above
- Findings reference the changed file's actual code
- The report reflects the default (changed-files) scope
