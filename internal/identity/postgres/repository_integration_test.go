//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"video-processor/internal/identity/domain"
	"video-processor/internal/identity/postgres"
	"video-processor/internal/platform/postgrestest"
)

const migrationsDir = "../../../migrations"

func TestRepository_CreateAndFindByEmail(t *testing.T) {
	pool := postgrestest.Pool(t, migrationsDir)
	repo := postgres.NewRepository(pool)
	ctx := context.Background()

	user := &domain.User{
		ID: uuid.New(), Name: "Ada Lovelace", Email: "ada@example.com",
		PasswordHash: "hashed", CreatedAt: time.Now(),
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	found, err := repo.FindByEmail(ctx, "ada@example.com")
	if err != nil {
		t.Fatalf("FindByEmail() error = %v, want nil", err)
	}
	if found.ID != user.ID {
		t.Errorf("FindByEmail().ID = %v, want %v", found.ID, user.ID)
	}
}

func TestRepository_CreateDuplicateEmail_ReturnsErrEmailAlreadyExists(t *testing.T) {
	pool := postgrestest.Pool(t, migrationsDir)
	repo := postgres.NewRepository(pool)
	ctx := context.Background()

	user := &domain.User{ID: uuid.New(), Name: "Ada", Email: "dup@example.com", PasswordHash: "h", CreatedAt: time.Now()}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("setup Create() error = %v", err)
	}

	other := &domain.User{ID: uuid.New(), Name: "Ada Two", Email: "dup@example.com", PasswordHash: "h2", CreatedAt: time.Now()}
	err := repo.Create(ctx, other)

	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Errorf("Create() error = %v, want %v", err, domain.ErrEmailAlreadyExists)
	}
}

func TestRepository_FindByEmail_UnknownEmail_ReturnsErrUserNotFound(t *testing.T) {
	pool := postgrestest.Pool(t, migrationsDir)
	repo := postgres.NewRepository(pool)

	_, err := repo.FindByEmail(context.Background(), "nobody@example.com")

	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("FindByEmail() error = %v, want %v", err, domain.ErrUserNotFound)
	}
}

func TestRepository_FindByID(t *testing.T) {
	pool := postgrestest.Pool(t, migrationsDir)
	repo := postgres.NewRepository(pool)
	ctx := context.Background()

	user := &domain.User{ID: uuid.New(), Name: "Ada", Email: "byid@example.com", PasswordHash: "h", CreatedAt: time.Now()}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("setup Create() error = %v", err)
	}

	found, err := repo.FindByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v, want nil", err)
	}
	if found.Email != "byid@example.com" {
		t.Errorf("FindByID().Email = %q, want %q", found.Email, "byid@example.com")
	}
}
