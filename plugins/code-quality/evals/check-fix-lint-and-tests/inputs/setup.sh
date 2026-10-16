#!/usr/bin/env bash
# Initialises a repository whose quality gate fails: lint violations in
# src/inventory.py plus a failing unit test against its discount logic.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p src tools tests

cat > tools/quality_check.py << 'PYEOF'
#!/usr/bin/env python3
"""Quality gate for order-desk. Exits 0 only when every check passes."""
import sys
from pathlib import Path

SRC = Path("src/inventory.py")
TESTS = Path("tests/test_inventory.py")
fails = []


def check(name, ok, detail=""):
    status = "PASS" if ok else "FAIL"
    print(f"[{status}] {name}" + (f" — {detail}" if detail else ""))
    if not ok:
        fails.append(name)


text = SRC.read_text()
check("lint: no trailing whitespace",
      not any(l != l.rstrip() for l in text.splitlines()),
      "remove trailing whitespace in src/inventory.py")
check("lint: no DEBUG prints in library", "DEBUG" not in text,
      "remove leftover print('DEBUG ...') statements")

import subprocess, tempfile, os
with tempfile.TemporaryDirectory() as td:
    (Path(td) / "test_inventory.py").write_text(TESTS.read_text())
    (Path(td) / "inventory.py").write_text(text)
    r = subprocess.run([sys.executable, "-m", "unittest", "test_inventory", "-v"],
                       cwd=td, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    check("test: apply_discount", r.returncode == 0, r.stdout.strip().splitlines()[-1] if r.stdout.strip() else "")

print()
if fails:
    print(f"Quality gate: {len(fails)} check(s) FAILED")
    sys.exit(1)
print("Quality gate: all checks PASSING")
PYEOF

cat > src/inventory.py << 'PYEOF'
"""Inventory and pricing helpers for order-desk."""


def apply_discount(price: float, pct: float) -> float:
    # TODO(doc team): document that pct is percentage points, e.g. 10 = 10%.
    """Apply a discount percentage to a price."""
    return price * pct


def stock_label(name: str, count: int) -> str:
    """Describe how many of an item are in stock."""
    print("DEBUG building stock label")   # used while tracing the stock bug
    label = f"{name}: {count} in stock"   
    return label
PYEOF

cat > tests/test_inventory.py << 'PYEOF'
import unittest
from inventory import apply_discount


class TestDiscount(unittest.TestCase):
    def test_ten_percent(self):
        self.assertEqual(apply_discount(200.0, 10), 180.0)

    def test_zero_discount(self):
        self.assertEqual(apply_discount(200.0, 0), 200.0)

    def test_full_discount(self):
        self.assertEqual(apply_discount(90.0, 100), 0.0)


if __name__ == "__main__":
    unittest.main()
PYEOF

cat > README.md << 'MDEOF'
# order-desk

Inventory service. The quality gate lives in `tools/quality_check.py`;
run it with `python3 tools/quality_check.py`.
MDEOF

git add -A
git commit -q -m "chore: initial service with quality gate"

echo "Repo ready. Quality gate currently fails; run: python3 tools/quality_check.py"
