package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
)

type User struct {
	ID           uuid.UUID
	Name         string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type Repository interface {
	Create(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
}

// TokenClaims is the domain's own view of a validated token — issuer
// implementations (e.g. internal/identity/jwt) translate their library's
// claim type into this, so the application layer never depends on a
// specific JWT library.
type TokenClaims struct {
	UserID string
	Email  string
}

// TokenIssuer is the port for issuing and validating session tokens.
type TokenIssuer interface {
	Issue(user *User) (string, error)
	Parse(tokenString string) (*TokenClaims, error)
}
