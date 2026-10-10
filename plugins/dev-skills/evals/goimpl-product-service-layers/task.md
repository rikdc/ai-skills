# Implement the Create Product Handler and Service Layers

## Background

You are picking up a pending change on the catalog API, a small Go backend
that serves a product catalog. The repository already contains the domain
models, the repository layer (an in-memory implementation is used in place
of PostgreSQL in this checkout), HTTP response helpers, and a stubbed
handler. The business logic service and the real handler body were never
written, so `POST /products` currently answers `501 Not Implemented`.

The team follows the house Go conventions documented in the
go-backend-implementor skill: handlers only decode requests and encode
responses, service constructors return unexported structs behind their
exported interface, all I/O functions take `context.Context` as their first
parameter, errors are sentinel errors wrapped with `%w` as they cross
layers, logging is structured (`zap`), and tests are table-driven with
testify and named subtests.

Run the setup script to prepare the repository in your working directory:

```bash
bash inputs/setup.sh
```

This creates a git repository with the scaffold described above and commits
it as the baseline.

## Your Task

1. Review the fixture: `internal/model/product.go`,
   `internal/repository/product.go`, `internal/handler/product.go`,
   `internal/handler/respond.go`, and `internal/service/product.go`
   (which currently declares only the `ProductService` interface and the
   validation sentinel errors). Read the house patterns for the service
   layer, constructor conventions, error handling, and table-driven tests
   before writing anything.
2. Implement the service layer in `internal/service/product.go`: add the
   unexported `productService` struct (repository and logger as
   interface-typed/dependency fields), a house-convention constructor
   `NewProductService` that returns the `ProductService` interface, and a
   `CreateProduct` method that validates the request (non-empty SKU, price
   greater than zero — wrap the matching sentinel error with `%w`), builds
   the `model.Product` (generate the ID in the service), persists it through
   `ProductRepository.Create` with the context propagated, and wraps any
   repository failure with context using `%w`.
3. Replace the stub body of `ProductHandler.Create` in
   `internal/handler/product.go`: decode the JSON body (log the failure with
   the structured logger and answer `400` via `writeError` on error), call
   the service with `r.Context()`, log any service failure with the
   structured logger and answer with an error response via `writeError`, and
   on success write the created product with status `201` via `writeJSON`.
   The handler must remain decode/encode only — no business logic, no
   repository access, no ID generation in the handler.
4. Write `internal/service/product_test.go` as a table-driven test using
   testify (`require`/`assert`) with named `t.Run` subtests, covering at
   least: a validation failure, the duplicate-SKU failure, and a successful
   creation against the in-memory repository.
5. Add godoc comments to every exported symbol you add or change.
6. **This environment has no Go toolchain and no network access: do not run
   `go build`, `go test`, `go vet`, or `go fmt`** — the dry-run equivalent is
   to record the exact commands you would run before submitting and what you
   expect each to show in a file named `VERIFICATION.md`. Do not claim any
   command was run or passed; state plainly that they were not run in this
   environment.

## Output Specification

After you finish, the repository should contain:

- `internal/service/product.go` — the implemented service (unexported struct
  behind the existing `ProductService` interface, house constructor,
  validation and error wrapping)
- `internal/handler/product.go` — the real `Create` handler replacing the
  `501` stub
- `internal/service/product_test.go` — the table-driven testify test
- `VERIFICATION.md` — the commands you would run and their expected results,
  without claiming they were executed
