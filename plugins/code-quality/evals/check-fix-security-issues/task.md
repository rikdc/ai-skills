# Fix Security Vulnerabilities in the Payment Module

## Background

The **paygate** repository is scheduled for a security review tomorrow. The
payment module shipped with three kinds of problems that our own quality gate
now flag: a hardcoded credential, an `eval()` over user-controlled input, and
a SQL statement built by string concatenation. We run `/dev:check` on the repo
to fix problems, not just list them, and no network access is available
locally — only what is in the working tree.

Start with `bash setup.sh` to create the repository state.

## Your Task

1. Run the quality gate (`python3 tools/quality_check.py`) and see which
   security checks fail.
2. Fix each problem in the product code:
   - Remove the hardcoded token and read the credential from the
     `PAYGATE_TOKEN` environment variable at runtime.
   - Replace the `eval()` call with the safe expression evaluator that is
     already provided in `src/safe_calc.py`.
   - Build the SQL query with a parameterised query (`?` placeholder) instead
     of concatenating user input.
3. Do not delete tests or gate checks to make things pass, and do not add
   new dependencies.
4. Record each fix in `fix-report.md`: file, the vulnerability, and the fix.

## Output Specification

- `python3 tools/quality_check.py` exits 0 with every security check passing
- `src/safe_calc.py` remains unchanged (use its helper, don't rewrite it)
- `fix-report.md` lists each vulnerability and its fix
