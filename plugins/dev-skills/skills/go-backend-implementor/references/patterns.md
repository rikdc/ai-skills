# House Go Patterns

A few concrete house conventions deviate from idiomatic-Go defaults; everything else follows the standard library and community practice (context-first I/O, sentinel errors with `%w` wrapping, `errors.Is`).

## Constructor Pattern

A deliberate house override of "accept interfaces, return structs". Because it is not what most Go reviewers expect, apply it only as instructed here — never "correct" it toward interface-typed returns elsewhere:

```go
// House convention: constructors return unexported structs as their interface.
func NewUserService(repo UserRepository, logger *zap.Logger) UserService {
    return &userService{repo: repo, logger: logger}
}
```

House style notes: structs stay unexported behind the interface (`userService`), the interface carries the plain exported name, dependencies are interface-typed fields (`repo UserRepository`), and code calls the interface, not the concrete type.

## Table-Driven Tests

House convention: table-driven tests using testify (`require`/`assert`) with subtests, rather than stdlib-only tests. Prefer `tt.expected.Equal(result)` style over struct equality where types support it (e.g. `decimal.Decimal` compare by value, not pointer).

```go
func TestCalculateTotal(t *testing.T) {
    tests := []struct {
        name     string
        items    []Item
        expected decimal.Decimal
        wantErr  bool
    }{
        {
            name:     "empty cart",
            items:    []Item{},
            expected: decimal.Zero,
            wantErr:  false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := CalculateTotal(tt.items)
            if tt.wantErr {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            assert.True(t, tt.expected.Equal(result))
        })
    }
}
```

## Service Layer Pattern

```go
// HTTP handler layer: only request decode/response encode, no business logic
type UserHandler struct {
    service UserService
    logger  *zap.Logger
}

func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    var req CreateUserRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.logger.Error("invalid request", zap.Error(err))
        writeError(w, http.StatusBadRequest, "Invalid request body")
        return
    }

    user, err := h.service.CreateUser(ctx, req)
    if err != nil {
        h.logger.Error("failed to create user", zap.Error(err))
        writeError(w, http.StatusInternalServerError, "Failed to create user")
        return
    }

    writeJSON(w, http.StatusCreated, user)
}

// Service: Business logic
type userService struct {
    repo   UserRepository
    bus    EventBus
    logger *zap.Logger
}

func (s *userService) CreateUser(ctx context.Context, req CreateUserRequest) (*User, error) {
    if err := s.validateUser(req); err != nil {
        return nil, fmt.Errorf("validation failed: %w", err)
    }

    user := &User{
        ID:    uuid.New(),
        Email: req.Email,
        Name:  req.Name,
    }

    if err := s.repo.Create(ctx, user); err != nil {
        return nil, fmt.Errorf("failed to create user: %w", err)
    }

    event := UserCreatedEvent{UserID: user.ID, Email: user.Email}
    if err := s.bus.Publish(ctx, "users.user_created", event); err != nil {
        s.logger.Warn("failed to publish event", zap.Error(err))
    }

    return user, nil
}
```

## Error Handling

```go
// Define sentinel errors
var (
    ErrUserNotFound   = errors.New("user not found")
    ErrInvalidEmail   = errors.New("invalid email address")
    ErrDuplicateEmail = errors.New("email already exists")
)

// Wrap errors with context
func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (*User, error) {
    user, err := s.repo.GetByID(ctx, id)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, ErrUserNotFound
        }
        return nil, fmt.Errorf("failed to query user %s: %w", id, err)
    }
    return user, nil
}
```
