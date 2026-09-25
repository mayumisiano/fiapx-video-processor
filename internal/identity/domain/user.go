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

// TokenIssuer is the port for issuing session tokens. Identity only issues
// — verifying a token is a stateless crypto operation any service can do
// with the shared secret, so it lives in internal/platform/jwt instead
// (see docs/adr/0007).
type TokenIssuer interface {
	Issue(user *User) (string, error)
}
