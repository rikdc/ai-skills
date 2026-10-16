#!/usr/bin/env bash
# Workspace: finished parent task awaiting the completion protocol.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p tasks src/audit
cat > tasks/tasks-prd-audit-export.md << 'EOF'
# Task list for Audit Export

## Relevant Files

- `src/audit/export.py` - CSV export of the audit event stream.
- `src/audit/export.test.py` - Tests for the exporter.

## Tasks

- [ ] 1.0 Audit CSV exporter
  - [x] 1.1 Implement event-to-CSV serializer
  - [x] 1.2 Add streaming download endpoint
  - [x] 1.3 Cover serializer with unit tests
- [ ] 2.0 Retention pruning job
  - [ ] 2.1 Prune events older than 180 days
EOF

cat > src/audit/export.py << 'EOF'
"""CSV export of the audit event stream."""


def row(event: dict) -> str:
    keys = ",".join(event)
    values = ",".join(str(event[k]) for k in event)
    return f"{keys}\n{values}"
EOF

cat > src/audit/export.test.py << 'EOF'
from src.audit.export import row


def test_row():
    assert "a,b" in row({"a": "1", "b": "2"})
EOF

cat > ci.sh << 'EOF'
#!/usr/bin/env bash
# Sandbox test-suite stand-in. Always passes on this fixture.
set -euo pipefail
echo "all tests passed" > ci-result.txt
exit 0
EOF
chmod +x ci.sh

cat > notes.tmp.md << 'EOF'
scratch: consider streaming gzip. Decide before release.
EOF

echo "Workspace ready: parent 1.0 done in code, protocol not yet run; notes.tmp.md littering the tree."
