#!/usr/bin/env bash
# Materialises the ledger service fixture: a concrete Postgres ledger store,
# an audit consumer that needs to write to it, and a note nudging toward a
# fat producer-side interface. Files are written into the current working
# directory; run from a fresh scratch directory.
set -euo pipefail

git init -q -b main
git config user.email "dev@example.com"
git config user.name "Dev"

mkdir -p internal/ledger internal/audit

cat > go.mod << 'EOF'
module example.com/ledger

go 1.22
EOF

cat > internal/ledger/ledger.go << 'EOF'
package ledger

import (
	"context"
	"database/sql"
	"time"
)

// PostgresLedger is the concrete append-only ledger store.
type PostgresLedger struct {
	db *sql.DB
}

func NewPostgresLedger(db *sql.DB) *PostgresLedger {
	return &PostgresLedger{db: db}
}

func (l *PostgresLedger) AppendEntry(ctx context.Context, accountID string, amountCents int64, memo string, at time.Time) error {
	_, err := l.db.ExecContext(ctx,
		`INSERT INTO ledger_entries (account_id, amount_cents, memo, recorded_at) VALUES ($1, $2, $3, $4)`,
		accountID, amountCents, memo, at)
	if err != nil {
		return err
	}
	return nil
}

func (l *PostgresLedger) AppendEntries(ctx context.Context, entries []Entry) error {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range entries {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO ledger_entries (account_id, amount_cents, memo, recorded_at) VALUES ($1, $2, $3, $4)`,
			e.AccountID, e.AmountCents, e.Memo, e.At)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (l *PostgresLedger) Balance(ctx context.Context, accountID string) (int64, error) {
	var balance int64
	err := l.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount_cents), 0) FROM ledger_entries WHERE account_id = $1`,
		accountID).Scan(&balance)
	return balance, err
}

func (l *PostgresLedger) Entries(ctx context.Context, accountID string, limit int) ([]Entry, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT account_id, amount_cents, memo, recorded_at FROM ledger_entries WHERE account_id = $1 ORDER BY recorded_at DESC LIMIT $2`,
		accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.AccountID, &e.AmountCents, &e.Memo, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (l *PostgresLedger) CloseAccounts(ctx context.Context, accountID string) error {
	_, err := l.db.ExecContext(ctx, `UPDATE accounts SET status = 'closed' WHERE id = $1`, accountID)
	return err
}

func (l *PostgresLedger) PurgeOldEntries(ctx context.Context, before time.Time) (int64, error) {
	res, err := l.db.ExecContext(ctx, `DELETE FROM ledger_entries WHERE recorded_at < $1`, before)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return n, err
}

func (l *PostgresLedger) HealthCheck(ctx context.Context) error {
	return l.db.PingContext(ctx)
}

type Entry struct {
	AccountID  string
	AmountCents int64
	Memo       string
	At         time.Time
}
EOF

cat > internal/audit/audit.go << 'EOF'
package audit

import (
	"context"
	"log/slog"
	"time"
)

// Event is one auditable action worth recording in the ledger.
type Event struct {
	AccountID   string
	AmountCents int64
	Memo        string
	At          time.Time
}

// Collector buffers audit events in memory and periodically flushes them to
// the ledger. The flush loop is NOT implemented yet — that is part of the
// design under review.
type Collector struct {
	queue chan Event
}

func NewCollector(buffer int) *Collector {
	return &Collector{queue: make(chan Event, buffer)}
}

func (c *Collector) Record(ctx context.Context, ev Event) error {
	select {
	case c.queue <- ev:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// FlushLoop is a stub. The team asked for advice on how to run it before we
// write it.
func (c *Collector) FlushLoop() {
	// TODO: drain c.queue into the ledger every 5 seconds.
	_ = slog.Default()
}
EOF

cat > TEAM_NOTES.md << 'EOF'
# Notes from the last planning meeting

The audit package needs to write entries into the ledger. To keep things
testable, someone proposed we define a `LedgerStore` interface **in the
`internal/ledger` package** listing every method `PostgresLedger` has
(AppendEntry, AppendEntries, Balance, Entries, CloseAccounts, PurgeOldEntries,
HealthCheck), so the ledger package owns its own abstraction and any consumer
can grab the whole thing for mocking.

Separately, the `Collector.FlushLoop` stub needs a home: it should drain the
in-memory queue and write to the ledger every 5 seconds, and it must shut down
cleanly when the service stops.
EOF

git add -A
git commit -q -m "chore: ledger service scaffold for audit integration review"

echo "Fixture ready:"
echo "  internal/ledger/ledger.go — concrete PostgresLedger with 7 methods"
echo "  internal/audit/audit.go   — audit Collector with FlushLoop stub"
echo "  TEAM_NOTES.md             — proposal to define a fat interface in the producer package"
echo "  git: main branch, 1 commit, working tree clean"
