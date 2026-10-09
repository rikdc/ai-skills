# Code Quality Standards

## Must-Have in Every Implementation

1. **Error Handling**: Every error must be handled or explicitly ignored with comment
2. **Tests**: Unit tests with >80% coverage, integration tests for external dependencies
3. **Documentation**: Public functions have godoc comments
4. **Logging**: Structured logs with correlation IDs for tracing
5. **Context**: All I/O operations accept context.Context as first parameter
6. **Interfaces**: Use small, focused interfaces for abstraction
7. **Validation**: Input validation at API boundaries
8. **Security**: No secrets in logs, validate and sanitize user input

## Code Review Checklist

Before submitting code, verify:

- [ ] All errors are handled properly
- [ ] Tests written and passing (`go test ./...`)
- [ ] Code formatted (`go fmt ./...` clean; check with `gofmt -l .` first)
- [ ] Linter passing (`golangci-lint run`)
- [ ] Race-free (`go test -race ./...` passes)
- [ ] Static analysis clean (`go vet ./...`)
- [ ] Documentation comments on public APIs
- [ ] No sensitive data in logs
- [ ] Resource cleanup (defer close, defer cancel)
- [ ] Context propagation throughout call chain
