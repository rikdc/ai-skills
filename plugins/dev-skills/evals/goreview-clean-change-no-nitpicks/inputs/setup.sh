#!/usr/bin/env bash
# Initialises the orders-service repo with a genuinely clean Go change
# staged for review. The point of the scenario: there is nothing blocking
# to find. The go toolchain is NOT required.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "dev@example.com"
git config user.name "Dev"

git checkout -q -b main
mkdir -p store
cat > store/user_store.go << 'GOEOF'
package store

import (
	"context"
	"database/sql"
	"fmt"
)

// UserStore reads user rows from the orders database.
type UserStore struct {
	db *sql.DB
}

// NewUserStore builds a store backed by the given database handle.
func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

// User is a single customer account row.
type User struct {
	ID    int
	Email string
}

// GetByID fetches one user by primary key.
func (s *UserStore) GetByID(ctx context.Context, id int) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		"SELECT id, email FROM users WHERE id = ?", id,
	).Scan(&u.ID, &u.Email)
	if err != nil {
		return User{}, fmt.Errorf("get user %d: %w", id, err)
	}
	return u, nil
}
GOEOF
git add store/user_store.go
git commit -q -m "feat: user store baseline"

# The change under review: an aggregation query plus a table-driven test.
# Staged, not yet committed, so `git diff --staged` shows exactly what the
# reviewer sees. Deliberately clean: context is propagated, the query is
# parameterized, errors are wrapped with %w, rows.Err is checked, the file
# is bounded by a named constant, and the pure helper is table-tested.
cat > store/summary.go << 'GOEOF'
package store

import (
	"context"
	"database/sql"
	"fmt"
)

// maxSummaryUsers bounds a single summary run so one call cannot exhaust
// memory on very large tables.
const maxSummaryUsers = 1000

// UserSummary aggregates the order count for one user.
type UserSummary struct {
	UserID int
	Orders int
}

// SummarizeUsers returns the most active users by order count, at most
// maxSummaryUsers of them.
func SummarizeUsers(ctx context.Context, db *sql.DB) ([]UserSummary, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT user_id, COUNT(*) FROM orders GROUP BY user_id ORDER BY COUNT(*) DESC LIMIT ?",
		maxSummaryUsers,
	)
	if err != nil {
		return nil, fmt.Errorf("query user summaries: %w", err)
	}
	defer rows.Close()

	summaries := make([]UserSummary, 0, maxSummaryUsers)
	for rows.Next() {
		var s UserSummary
		if err := rows.Scan(&s.UserID, &s.Orders); err != nil {
			return nil, fmt.Errorf("scan user summary: %w", err)
		}
		summaries = append(summaries, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user summaries: %w", err)
	}
	return summaries, nil
}

// Describe renders one summary line for the admin report.
func Describe(s UserSummary) string {
	return fmt.Sprintf("user %d has %d orders", s.UserID, s.Orders)
}
GOEOF
cat > store/summary_test.go << 'GOEOF'
package store

import (
	"testing"
)

func TestDescribe(t *testing.T) {
	tests := []struct {
		name string
		in   UserSummary
		want string
	}{
		{"single order", UserSummary{UserID: 7, Orders: 1}, "user 7 has 1 orders"},
		{"many orders", UserSummary{UserID: 9, Orders: 40}, "user 9 has 40 orders"},
		{"zero orders", UserSummary{UserID: 2, Orders: 0}, "user 2 has 0 orders"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Describe(tt.in); got != tt.want {
				t.Errorf("Describe(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
GOEOF
git add store/summary.go store/summary_test.go

echo "Repo ready. Staged (uncommitted) change: new files store/summary.go and store/summary_test.go."
echo "Review it with: git diff --staged"
