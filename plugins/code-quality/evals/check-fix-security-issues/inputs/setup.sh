#!/usr/bin/env bash
# Initialises a repository whose payment module fails the security portion of
# the quality gate: hardcoded token, eval() on user input, SQL concat.
set -euo pipefail

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p src tools tests

cat > src/safe_calc.py << 'PYEOF'
"""Safe arithmetic evaluator, provided for payments to use."""


def safe_eval(expr: str) -> int:
    """Evaluate an arithmetic expression consisting of digits and + - * / only."""
    import ast, operator
    allowed = {ast.Add: operator.add, ast.Sub: operator.sub,
               ast.Mult: operator.mul, ast.Div: operator.floordiv}
    def ev(node):
        if isinstance(node, ast.Expression):
            return ev(node.body)
        if isinstance(node, ast.BinOp):
            return allowed[type(node.op)](ev(node.left), ev(node.right))
        if isinstance(node, ast.Constant) and isinstance(node.value, int):
            return node.value
        raise ValueError("disallowed expression")
    return ev(ast.parse(expr, mode="eval"))
PYEOF

cat > src/payments.py << 'PYEOF'
"""Payment processing for paygate."""
import sqlite3

API_TOKEN = "sk-live-9f2e77c1b4d84a01"


def auth() -> str:
    """Return the API credential used to authenticate with paygate."""
    return API_TOKEN


def compute_charge(amount_expr: str) -> int:
    """Compute a charge from a user-supplied arithmetic expression."""
    # The old interface accepted expressions typed directly by users.
    return eval(amount_expr)


def statement(user: str) -> str:
    """Build the statement query for a user."""
    return "SELECT amount FROM ledger WHERE user = '" + user + "'"


def run(user: str, amount_expr: str) -> None:
    """Process a payment and print the receipt line."""
    token = auth()
    amount = compute_charge(amount_expr)
    db = sqlite3.connect(":memory:")
    db.execute("CREATE TABLE ledger (user TEXT, amount INT)")
    db.executemany("INSERT INTO ledger VALUES (?, ?)",
                   [(user, amount), (user, 5)])
    rows = db.execute(statement(user) % (user,)).fetchall()
    db.close()
    print(f"receipt user={user} amount={sum(r[0] for r in rows)}")


if __name__ == "__main__":
    run("alice", "4 + 4")
PYEOF

cat > tests/test_payments.py << 'PYEOF'
import unittest
from payments import compute_charge, statement


class TestPayments(unittest.TestCase):
    def test_compute_charge(self):
        self.assertEqual(compute_charge("2 + 2 * 3"), 8)

    def test_statement_has_placeholder(self):
        self.assertIn("?", statement("alice"))


if __name__ == "__main__":
    unittest.main()
PYEOF

cat > tools/quality_check.py << 'PYEOF'
#!/usr/bin/env python3
"""Quality gate for paygate. Exits 0 only when every check passes."""
import sys
from pathlib import Path
import re

FILES = ["src/payments.py"]
text = "\n".join(Path(f).read_text() for f in FILES)
fails = []


def check(name, ok, detail=""):
    status = "PASS" if ok else "FAIL"
    print(f"[{status}] {name}" + (f" — {detail}" if detail else ""))
    if not ok:
        fails.append(name)


check("sec: no hardcoded credential", "sk-live-" not in text,
      "remove the embedded token; read PAYGATE_TOKEN from the environment")
check("sec: no eval of user input", not re.search(r"\beval\s*\(", text),
      "use src/safe_calc.py's safe_eval instead")
check("sec: sql parameterised",
      "?" in text and not re.search(r"SELECT[^'\n]*'\s*\+|=\s*'", text),
      "use a ? placeholder with a parameter tuple instead of string concatenation")

import subprocess, tempfile
with tempfile.TemporaryDirectory() as td:
    for name in ("payments.py", "safe_calc.py"):
        (Path(td) / name).write_text(Path("src") / name and Path("src", name).read_text())
    (Path(td) / "test_payments.py").write_text(Path("tests/test_payments.py").read_text())
    r = subprocess.run([sys.executable, "-m", "unittest", "test_payments"],
                       cwd=td, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    check("test: payments unit tests", r.returncode == 0, r.stdout.strip().splitlines()[-1])

print()
if fails:
    print(f"Quality gate: {len(fails)} check(s) FAILED")
    sys.exit(1)
print("Quality gate: all checks PASSING")
PYEOF

cat > README.md << 'MDEOF'
# paygate

Payment service. Gate: `python3 tools/quality_check.py`.
MDEOF

git add -A
git commit -q -m "chore: payment module scaffold"

echo "Repo ready. Gate fails security checks; run: python3 tools/quality_check.py"
