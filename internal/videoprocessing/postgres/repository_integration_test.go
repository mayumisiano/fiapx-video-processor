//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"video-processor/internal/platform/postgrestest"
	"video-processor/internal/videoprocessing/domain"
	"video-processor/internal/videoprocessing/postgres"
)

const migrationsDir = "../../../migrations/video"

func TestRepository_CreateAndFindByID(t *testing.T) {
	pool := postgrestest.Pool(t, migrationsDir)
	userID := uuid.New()
	repo := postgres.NewRepository(pool)
	ctx := context.Background()

	req := domain.NewProcessingRequest(userID, "ada@example.com", domain.VideoMetadata{
		OriginalName: "movie.mp4", SizeBytes: 2048, Format: "mp4",
	}, "videos/"+uuid.NewString()+"/movie.mp4")

	if err := repo.Create(ctx, req); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	found, err := repo.FindByID(ctx, req.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v, want nil", err)
	}
	if found.Metadata.OriginalName != "movie.mp4" || found.Status != domain.StatusPending {
		t.Errorf("FindByID() = %+v, want OriginalName=movie.mp4, Status=PENDING", found)
	}
}

func TestRepository_FindByID_Unknown_ReturnsErrRequestNotFound(t *testing.T) {
	pool := postgrestest.Pool(t, migrationsDir)
	repo := postgres.NewRepository(pool)

	_, err := repo.FindByID(context.Background(), uuid.New())

	if !errors.Is(err, domain.ErrRequestNotFound) {
		t.Errorf("FindByID() error = %v, want %v", err, domain.ErrRequestNotFound)
	}
}

func TestRepository_Update_PersistsStatusAndResult(t *testing.T) {
	pool := postgrestest.Pool(t, migrationsDir)
	userID := uuid.New()
	repo := postgres.NewRepository(pool)
	ctx := context.Background()

	req := domain.NewProcessingRequest(userID, "ada@example.com", domain.VideoMetadata{OriginalName: "movie.mp4", Format: "mp4"}, "videos/x/movie.mp4")
	if err := repo.Create(ctx, req); err != nil {
		t.Fatalf("setup Create() error = %v", err)
	}

	if err := req.StartProcessing(); err != nil {
		t.Fatalf("setup StartProcessing() error = %v", err)
	}
	if err := repo.Update(ctx, req); err != nil {
		t.Fatalf("Update() (processing) error = %v, want nil", err)
	}

	if err := req.CompleteProcessing(domain.ProcessingResult{FrameCount: 7, PackageReference: "results/x.zip"}); err != nil {
		t.Fatalf("setup CompleteProcessing() error = %v", err)
	}
	if err := repo.Update(ctx, req); err != nil {
		t.Fatalf("Update() (completed) error = %v, want nil", err)
	}

	found, err := repo.FindByID(ctx, req.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v, want nil", err)
	}
	if found.Status != domain.StatusCompleted {
		t.Errorf("Status = %q, want %q", found.Status, domain.StatusCompleted)
	}
	if found.Result == nil || found.Result.FrameCount != 7 || found.Result.PackageReference != "results/x.zip" {
		t.Errorf("Result = %+v, want FrameCount=7 PackageReference=results/x.zip", found.Result)
	}
}

func TestRepository_ListByUser_OrdersByCreatedAtDescending(t *testing.T) {
	pool := postgrestest.Pool(t, migrationsDir)
	userID := uuid.New()
	repo := postgres.NewRepository(pool)
	ctx := context.Background()

	first := domain.NewProcessingRequest(userID, "ada@example.com", domain.VideoMetadata{OriginalName: "first.mp4", Format: "mp4"}, "videos/1")
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("setup Create(first) error = %v", err)
	}
	second := domain.NewProcessingRequest(userID, "ada@example.com", domain.VideoMetadata{OriginalName: "second.mp4", Format: "mp4"}, "videos/2")
	second.CreatedAt = first.CreatedAt.Add(time.Second)
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("setup Create(second) error = %v", err)
	}

	requests, err := repo.ListByUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListByUser() error = %v, want nil", err)
	}
	if len(requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(requests))
	}
	if requests[0].ID != second.ID {
		t.Errorf("requests[0].ID = %v, want the most recently created request %v", requests[0].ID, second.ID)
	}
}
