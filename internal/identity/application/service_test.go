package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"video-processor/internal/identity/application"
	"video-processor/internal/identity/domain"
)

type fakeRepo struct {
	byEmail map[string]*domain.User
	byID    map[uuid.UUID]*domain.User
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byEmail: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}}
}

func (r *fakeRepo) Create(ctx context.Context, user *domain.User) error {
	if _, exists := r.byEmail[user.Email]; exists {
		return domain.ErrEmailAlreadyExists
	}
	r.byEmail[user.Email] = user
	r.byID[user.ID] = user
	return nil
}

func (r *fakeRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	if u, ok := r.byEmail[email]; ok {
		return u, nil
	}
	return nil, domain.ErrUserNotFound
}

func (r *fakeRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	if u, ok := r.byID[id]; ok {
		return u, nil
	}
	return nil, domain.ErrUserNotFound
}

type fakeIssuer struct{}

func (fakeIssuer) Issue(user *domain.User) (string, error) {
	return "token-for-" + user.ID.String(), nil
}

func (fakeIssuer) Parse(tokenString string) (*domain.TokenClaims, error) {
	if tokenString == "" {
		return nil, domain.ErrInvalidToken
	}
	return &domain.TokenClaims{UserID: tokenString}, nil
}

func TestRegister_NewEmail_ReturnsUserAndToken(t *testing.T) {
	svc := application.NewService(newFakeRepo(), fakeIssuer{})

	result, err := svc.Register(context.Background(), "Ada", "ada@example.com", "s3cret123")

	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	if result.User.Email != "ada@example.com" {
		t.Errorf("User.Email = %q, want ada@example.com", result.User.Email)
	}
	if result.User.PasswordHash == "s3cret123" {
		t.Errorf("PasswordHash was stored in plaintext")
	}
	if result.Token == "" {
		t.Errorf("Token is empty, want non-empty")
	}
}

func TestRegister_DuplicateEmail_ReturnsErrEmailAlreadyExists(t *testing.T) {
	repo := newFakeRepo()
	svc := application.NewService(repo, fakeIssuer{})
	ctx := context.Background()
	if _, err := svc.Register(ctx, "Ada", "ada@example.com", "s3cret123"); err != nil {
		t.Fatalf("setup Register() error = %v", err)
	}

	_, err := svc.Register(ctx, "Ada Two", "ada@example.com", "other-pass")

	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Errorf("Register() error = %v, want %v", err, domain.ErrEmailAlreadyExists)
	}
}

func TestLogin_CorrectPassword_ReturnsToken(t *testing.T) {
	repo := newFakeRepo()
	svc := application.NewService(repo, fakeIssuer{})
	ctx := context.Background()
	if _, err := svc.Register(ctx, "Ada", "ada@example.com", "s3cret123"); err != nil {
		t.Fatalf("setup Register() error = %v", err)
	}

	result, err := svc.Login(ctx, "ada@example.com", "s3cret123")

	if err != nil {
		t.Fatalf("Login() error = %v, want nil", err)
	}
	if result.Token == "" {
		t.Errorf("Token is empty, want non-empty")
	}
}

func TestLogin_WrongPassword_ReturnsErrInvalidCredentials(t *testing.T) {
	repo := newFakeRepo()
	svc := application.NewService(repo, fakeIssuer{})
	ctx := context.Background()
	if _, err := svc.Register(ctx, "Ada", "ada@example.com", "s3cret123"); err != nil {
		t.Fatalf("setup Register() error = %v", err)
	}

	_, err := svc.Login(ctx, "ada@example.com", "wrong-password")

	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("Login() error = %v, want %v", err, domain.ErrInvalidCredentials)
	}
}

func TestLogin_UnknownEmail_ReturnsErrInvalidCredentials(t *testing.T) {
	svc := application.NewService(newFakeRepo(), fakeIssuer{})

	_, err := svc.Login(context.Background(), "nobody@example.com", "whatever")

	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("Login() error = %v, want %v (should not leak ErrUserNotFound)", err, domain.ErrInvalidCredentials)
	}
}

func TestAuthenticate_DelegatesToIssuer(t *testing.T) {
	svc := application.NewService(newFakeRepo(), fakeIssuer{})

	claims, err := svc.Authenticate("some-token")

	if err != nil {
		t.Fatalf("Authenticate() error = %v, want nil", err)
	}
	if claims.UserID != "some-token" {
		t.Errorf("claims.UserID = %q, want %q", claims.UserID, "some-token")
	}
}
