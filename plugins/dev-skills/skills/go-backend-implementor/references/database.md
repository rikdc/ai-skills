# Database Patterns

PostgreSQL repository and transaction conventions.

## Repository Setup

```go
// House convention: unexported struct returned as its interface.
type repository struct {
    db  *sql.DB
    log *zap.Logger
}
```

## Queries

```go
// Package-level query constants keep SQL in one place and are inlined by
// database/sql without extra round-trips (no need for explicit Prepare).
const getUserQuery = `
    SELECT id, email, name, created_at
    FROM users
    WHERE id = $1
`

func (r *repository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
    var user User
    err := r.db.QueryRowContext(ctx, getUserQuery, id).Scan(
        &user.ID, &user.Email, &user.Name, &user.CreatedAt,
    )
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, ErrUserNotFound
        }
        return nil, fmt.Errorf("failed to query user: %w", err)
    }
    return &user, nil
}
```

## Transactions

```go
func (r *repository) Transfer(ctx context.Context, from, to uuid.UUID, amount decimal.Decimal) error {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer func() {
        if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
            r.log.Warn("rollback failed", zap.Error(err))
        }
    }()

    if err := r.debit(ctx, tx, from, amount); err != nil {
        return err
    }

    if err := r.credit(ctx, tx, to, amount); err != nil {
        return err
    }

    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }

    return nil
}
```

The deferred rollback is unconditional: once `Commit` succeeds, PostgreSQL has already decided the outcome, so the post-commit rollback returning `sql.ErrTxDone` is expected cleanup noise — never surface it as a failure. Only unexpected rollback errors (e.g. the connection dying mid-rollback) get logged.
