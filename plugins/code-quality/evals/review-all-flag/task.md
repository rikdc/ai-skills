# Review the Entire Codebase (--all)

## Background

Release prep in the **metrics** repository: reviewers run `/dev:review --all`
from the code-quality plugin for a comprehensive whole-repository analysis —
unlike the default run, which restricts itself to changed files. You are on
main with a clean tree; nothing is pending, so the full-repository review has
to stand on its own and must not claim to be a scoped change review.

Start with `bash setup.sh` to create the repository state.

## Your Task

1. Follow the `--all` behaviour: analyze the **entire repository codebase**,
   not just recent commits or modified files (there are none anyway).
2. Review every source file for performance, security, maintainability,
   standards compliance, and unhandled edge cases.
3. Write `review.md` in the command's structured report format:
   - `📊 Change Summary` — noting this is a full-repository analysis
   - `🔍 Priority Findings` — High, Medium, Low subsections
   - `⚠️ Edge Cases & Risks`
   - `✅ Recommendations`
4. Findings must reference real code. Known suspects include the module that
   hardcodes the placeholder service token and one that validates nothing —
   but verify everything yourself and grade by severity, not by file size.

## Output Specification

- `review.md` with the four sections, covering the full repository
- Security issues (e.g. the placeholder token) appear under High Priority
- Professional PR-review tone with concrete code references
