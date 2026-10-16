#!/usr/bin/env bash
# Initialises the inventory-service repo with a staged change that is
# presented as a pure cosmetic refactor but actually introduces a
# guaranteed index-out-of-range panic. The go toolchain is NOT required.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

git checkout -q -b main
mkdir -p inventory
cat > inventory/totals.go << 'GOEOF'
package inventory

// Item is one line of a stock count.
type Item struct {
	SKU   string
	Count int
}

// SumTotals adds up the counts of every counted item.
func SumTotals(items []Item) int {
	sum := 0
	for _, item := range items {
		sum += item.Count
	}
	return sum
}
GOEOF
cat > inventory/totals_test.go << 'GOEOF'
package inventory

import (
	"testing"
)

func TestSumTotals(t *testing.T) {
	tests := []struct {
		name  string
		items []Item
		want  int
	}{
		{"empty", nil, 0},
		{"one item", []Item{{SKU: "A1", Count: 3}}, 3},
		{"several items", []Item{{SKU: "A1", Count: 3}, {SKU: "B2", Count: 4}, {SKU: "C3", Count: 5}}, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SumTotals(tt.items); got != tt.want {
				t.Errorf("SumTotals(%v) = %d, want %d", tt.items, got, tt.want)
			}
		})
	}
}
GOEOF
git add inventory/
git commit -q -m "feat: stock count totals"

# The change under review: the teammate's "purely cosmetic" refactor.
# Staged, not yet committed, so `git diff --staged` shows exactly what the
# reviewer sees. The rename and doc comment are cosmetic; the rewritten
# loop condition `i <= len(items)` makes SumTotals panic with
# index out of range on every non-empty input, which the existing
# table-driven test would catch.
cat > inventory/totals.go << 'GOEOF'
package inventory

// Item is one line of a stock count.
type Item struct {
	SKU   string
	Count int
}

// SumTotals adds up the counts of every counted item. Keep the accumulated
// value in a clearly named accumulator rather than reusing the range
// variable, per the style pass.
func SumTotals(items []Item) int {
	total := 0
	for i := 0; i <= len(items); i++ {
		total += items[i].Count
	}
	return total
}
GOEOF
git add inventory/totals.go

echo "Repo ready. Staged (uncommitted) change: rewrite of SumTotals in inventory/totals.go."
echo "Review it with: git diff --staged"
