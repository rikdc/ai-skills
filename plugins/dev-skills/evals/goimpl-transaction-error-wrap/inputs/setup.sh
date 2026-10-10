#!/usr/bin/env bash
# Materialises the ledger service with a merged-but-flawed money-transfer
# change: the transaction and error handling violate the house Go rules the
# agent must fix, while the constructor in repository.go is a correct house
# convention the agent must leave alone. Everything is heredoc'd relative to
# the directory containing inputs/, so the script works regardless of the
# caller's cwd.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdir -p internal/model internal/repository internal/service

cat > go.mod << 'EOF'
module ledgerapi

go 1.22

require (
	github.com/google/uuid v1.6.0
	github.com/shopspring/decimal v1.4.0
	go.uber.org/zap v1.27.0
)
EOF

cat > internal/model/account.go << 'EOF'
// Package model contains the domain models of the ledger service.
package model

import (
	"time"

	"github.com/google/uuid"
)

// Account is a ledger account owned by a customer.
type Account struct {
	ID        uuid.UUID `json:"id"`
	Owner     string    `json:"owner"`
	Balance   string    `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
}
EOF

cat > internal/repository/repository.go << 'EOF'
// Package repository contains the data access layer of the ledger service.
package repository

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// LedgerRepository is the data-access interface for ledger operations.
type LedgerRepository interface {
	// Transfer moves amount between two accounts atomically.
	Transfer(ctx context.Context, from, to uuid.UUID, amount decimal.Decimal) error
	// GetBalance returns the current balance of an account.
	GetBalance(ctx context.Context, id uuid.UUID) (decimal.Decimal, error)
}

type postgresRepository struct {
	db  *sql.DB
	log *zap.Logger
}

// NewLedgerRepository returns the PostgreSQL-backed ledger repository.
// House convention: constructors return unexported structs behind their
// exported interface.
func NewLedgerRepository(db *sql.DB, log *zap.Logger) LedgerRepository {
	return &postgresRepository{db: db, log: log}
}
EOF

cat > internal/repository/ledger.go << 'EOF'
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Sentinel errors returned by the ledger repository.
var (
	// ErrAccountNotFound is returned when an account does not exist.
	ErrAccountNotFound = errors.New("account not found")
	// ErrInsufficientFunds is returned when the source account cannot cover the amount.
	ErrInsufficientFunds = errors.New("insufficient funds")
)

const (
	debitQuery  = `UPDATE accounts SET balance = balance - $1 WHERE id = $2 AND balance >= $1`
	creditQuery = `UPDATE accounts SET balance = balance + $1 WHERE id = $2`
)

// Transfer moves amount between two accounts atomically.
func (r *postgresRepository) Transfer(ctx context.Context, from, to uuid.UUID, amount decimal.Decimal) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil {
			err = fmt.Errorf("rollback failed: %w", rbErr)
		}
	}()

	if err := r.debit(ctx, tx, from, amount); err != nil {
		return err
	}

	if err := r.credit(ctx, tx, to, amount); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// debit withdraws amount from the source account inside tx.
func (r *postgresRepository) debit(ctx context.Context, tx *sql.Tx, id uuid.UUID, amount decimal.Decimal) error {
	res, err := tx.ExecContext(ctx, debitQuery, amount, id)
	if err != nil {
		return fmt.Errorf("failed to debit account %s: %v", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("failed to debit account %s: %v", id, ErrInsufficientFunds)
	}
	return nil
}

// credit deposits amount into the destination account inside tx.
func (r *postgresRepository) credit(ctx context.Context, tx *sql.Tx, id uuid.UUID, amount decimal.Decimal) error {
	if _, err := tx.ExecContext(ctx, creditQuery, amount, id); err != nil {
		return fmt.Errorf("failed to credit account %s: %v", id, err)
	}
	return nil
}

// GetBalance returns the current balance of an account.
func (r *postgresRepository) GetBalance(ctx context.Context, id uuid.UUID) (decimal.Decimal, error) {
	const getBalanceQuery = `SELECT balance FROM accounts WHERE id = $1`

	var balance decimal.Decimal
	err := r.db.QueryRowContext(ctx, getBalanceQuery, id).Scan(&balance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return decimal.Decimal{}, ErrAccountNotFound
		}
		return decimal.Decimal{}, fmt.Errorf("failed to query balance for account %s: %w", id, err)
	}
	return balance, nil
}
EOF

cat > internal/service/payment.go << 'EOF'
// Package service contains the business logic of the ledger service.
package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"ledgerapi/internal/repository"
)

// ErrPaymentRejected is returned to callers when a payment cannot be made.
var ErrPaymentRejected = errors.New("payment rejected")

// PaymentService is the business-logic interface for payments.
type PaymentService interface {
	// Transfer moves amount between two accounts.
	Transfer(ctx context.Context, from, to uuid.UUID, amount decimal.Decimal) error
}

type paymentService struct {
	repo repository.LedgerRepository
}

// NewPaymentService returns a payment service backed by the given repository.
func NewPaymentService(repo repository.LedgerRepository) PaymentService {
	return &paymentService{repo: repo}
}

// Transfer moves amount between two accounts, rejecting payments that the
// ledger cannot settle.
func (s *paymentService) Transfer(ctx context.Context, from, to uuid.UUID, amount decimal.Decimal) error {
	if err := s.repo.Transfer(ctx, from, to, amount); err != nil {
		if err == repository.ErrInsufficientFunds {
			return ErrPaymentRejected
		}
		return err
	}
	return nil
}
EOF

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"
git add -A
git commit -q -m "feat: atomic money transfer between ledger accounts"

echo "Ledger service ready in $ROOT"
echo "Baseline commit: $(git rev-parse --short HEAD)"
echo "Review targets: internal/repository/ledger.go, internal/service/payment.go"
echo "House convention to preserve: internal/repository/repository.go constructor."
