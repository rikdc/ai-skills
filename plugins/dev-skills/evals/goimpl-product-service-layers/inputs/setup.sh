#!/usr/bin/env bash
# Materialises the catalog API scaffold: domain models, an in-memory
# repository, HTTP response helpers, a stubbed product handler, and the
# ProductService interface awaiting its implementation. Everything is
# heredoc'd relative to the directory containing inputs/, so the script
# works regardless of the caller's cwd.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdir -p internal/model internal/repository internal/service internal/handler

cat > go.mod << 'EOF'
module catalogapi

go 1.22

require (
	github.com/google/uuid v1.6.0
	github.com/stretchr/testify v1.9.0
	go.uber.org/zap v1.27.0
)
EOF

cat > internal/model/product.go << 'EOF'
// Package model contains the domain models of the catalog API.
package model

import (
	"time"

	"github.com/google/uuid"
)

// Product is a product in the catalog.
type Product struct {
	ID         uuid.UUID `json:"id"`
	SKU        string    `json:"sku"`
	Name       string    `json:"name"`
	PriceCents int64     `json:"price_cents"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateProductRequest is the request body for creating a product.
type CreateProductRequest struct {
	SKU        string `json:"sku"`
	Name       string `json:"name"`
	PriceCents int64  `json:"price_cents"`
}
EOF

cat > internal/repository/product.go << 'EOF'
// Package repository contains the data access layer of the catalog API.
package repository

import (
	"context"
	"errors"
	"sync"

	"catalogapi/internal/model"
)

// ErrDuplicateSKU is returned when a product with the same SKU already exists.
var ErrDuplicateSKU = errors.New("duplicate SKU")

// ProductRepository is the data-access interface for products.
type ProductRepository interface {
	// Create persists a new product.
	Create(ctx context.Context, p *model.Product) error
}

type inMemoryProductRepository struct {
	mu    sync.Mutex
	bySKU map[string]model.Product
}

// NewInMemoryProductRepository returns an empty in-memory product repository.
func NewInMemoryProductRepository() ProductRepository {
	return &inMemoryProductRepository{bySKU: make(map[string]model.Product)}
}

// Create persists a new product, rejecting duplicate SKUs.
func (r *inMemoryProductRepository) Create(ctx context.Context, p *model.Product) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.bySKU[p.SKU]; ok {
		return ErrDuplicateSKU
	}
	r.bySKU[p.SKU] = *p
	return nil
}
EOF

cat > internal/service/product.go << 'EOF'
// Package service contains the business logic of the catalog API.
package service

import (
	"context"
	"errors"

	"catalogapi/internal/model"
)

// Sentinel errors returned by the product service.
var (
	// ErrInvalidSKU is returned when a product SKU fails validation.
	ErrInvalidSKU = errors.New("invalid SKU")
	// ErrInvalidPrice is returned when a product price fails validation.
	ErrInvalidPrice = errors.New("invalid price")
)

// ProductService is the business-logic interface for products.
type ProductService interface {
	// CreateProduct validates and persists a new product.
	CreateProduct(ctx context.Context, req model.CreateProductRequest) (*model.Product, error)
}
EOF

cat > internal/handler/respond.go << 'EOF'
// Package handler contains the HTTP handlers of the catalog API.
package handler

import (
	"encoding/json"
	"net/http"
)

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers are already sent; nothing more can be done about it.
		_ = err
	}
}

// writeError writes a plain-text error message with the given status code.
func writeError(w http.ResponseWriter, status int, msg string) {
	http.Error(w, msg, status)
}
EOF

cat > internal/handler/product.go << 'EOF'
package handler

import (
	"net/http"

	"go.uber.org/zap"

	"catalogapi/internal/service"
)

// ProductHandler serves the product endpoints of the catalog API.
type ProductHandler struct {
	service service.ProductService
	logger  *zap.Logger
}

// NewProductHandler returns a handler for the product endpoints.
func NewProductHandler(service service.ProductService, logger *zap.Logger) *ProductHandler {
	return &ProductHandler{service: service, logger: logger}
}

// Create handles POST /products.
func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented")
}
EOF

git init -q
git config user.email "dev@example.com"
git config user.name "Dev"
git add -A
git commit -q -m "chore: catalog API scaffold with stubbed product handler"

echo "Catalog API scaffold ready in $ROOT"
echo "Baseline commit: $(git rev-parse --short HEAD)"
echo "Waiting for: internal/service/product.go implementation, real handler body, service test."
