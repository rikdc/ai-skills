#!/usr/bin/env bash
# Initialises the orders-service repo with a flawed Go change staged for
# review. The go toolchain is NOT required: the review is done by reading.
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

# The change under review: a new CSV export endpoint backend. Staged, not
# yet committed, so `git diff --staged` shows exactly what the reviewer sees.
cat > store/export.go << 'GOEOF'
package store

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"sync"
	"time"
)

// ExportUserOrders writes every order for the given user to a CSV file.
// It backs the admin export endpoint and receives the raw query string
// values from the admin UI.
func ExportUserOrders(db *sql.DB, userID int, email, path string) error {
	query := fmt.Sprintf(
		"SELECT id, total, created_at FROM orders WHERE user_id = %d AND email = '%s'",
		userID, email,
	)
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create export file: %w", err)
	}
	w := csv.NewWriter(f)

	var wg sync.WaitGroup
	records := make([][]string, 0)
	for rows.Next() {
		var (
			id      int
			total   float64
			created time.Time
		)
		if err := rows.Scan(&id, &total, &created); err != nil {
			return err
		}
		wg.Add(1)
		go func(id int, total float64) {
			defer wg.Done()
			label := lookupLabel(db, id)
			records = append(records, []string{
				fmt.Sprintf("%d", id),
				fmt.Sprintf("%.2f", total),
				label,
			})
		}(id, total)
	}
	wg.Wait()
	w.WriteAll(records)
	return w.Error()
}

// lookupLabel resolves the human-readable label for one order.
func lookupLabel(db *sql.DB, orderID int) string {
	var label string
	db.QueryRow("SELECT label FROM order_labels WHERE order_id = ?", orderID).Scan(&label)
	return label
}
GOEOF
git add store/export.go

echo "Repo ready. Staged (uncommitted) change: new file store/export.go."
echo "Review it with: git diff --staged"
