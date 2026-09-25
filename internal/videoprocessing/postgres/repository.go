package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"video-processor/internal/videoprocessing/domain"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, req *domain.ProcessingRequest) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO processing_requests
			(id, user_id, user_email, original_filename, format, size_bytes, video_storage_key, status, attempts, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		req.ID, req.UserID, req.UserEmail, req.Metadata.OriginalName, req.Metadata.Format, req.Metadata.SizeBytes,
		req.VideoStorageKey, req.Status, req.Attempts, req.CreatedAt, req.UpdatedAt,
	)
	return err
}

func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (*domain.ProcessingRequest, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, user_id, user_email, original_filename, format, size_bytes, video_storage_key,
			result_storage_key, frame_count, status, failure_reason, attempts, created_at, updated_at
		FROM processing_requests WHERE id = $1`,
		id,
	)
	req, err := scanRequest(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrRequestNotFound
		}
		return nil, err
	}
	return req, nil
}

func (r *Repository) ListByUser(ctx context.Context, userID uuid.UUID) ([]*domain.ProcessingRequest, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, user_email, original_filename, format, size_bytes, video_storage_key,
			result_storage_key, frame_count, status, failure_reason, attempts, created_at, updated_at
		FROM processing_requests WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []*domain.ProcessingRequest
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, req)
	}
	return requests, rows.Err()
}

func (r *Repository) Update(ctx context.Context, req *domain.ProcessingRequest) error {
	var packageReference *string
	var frameCount *int
	if req.Result != nil {
		packageReference = &req.Result.PackageReference
		frameCount = &req.Result.FrameCount
	}

	var failureReason *string
	if req.FailureReason != "" {
		reason := string(req.FailureReason)
		failureReason = &reason
	}

	_, err := r.pool.Exec(ctx,
		`UPDATE processing_requests SET
			status = $1, failure_reason = $2, attempts = $3, result_storage_key = $4, frame_count = $5, updated_at = $6
		WHERE id = $7`,
		req.Status, failureReason, req.Attempts, packageReference, frameCount, req.UpdatedAt, req.ID,
	)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRequest(s scanner) (*domain.ProcessingRequest, error) {
	var req domain.ProcessingRequest
	var resultStorageKey, failureReason *string
	var frameCount *int

	err := s.Scan(
		&req.ID, &req.UserID, &req.UserEmail, &req.Metadata.OriginalName, &req.Metadata.Format, &req.Metadata.SizeBytes,
		&req.VideoStorageKey, &resultStorageKey, &frameCount, &req.Status, &failureReason, &req.Attempts,
		&req.CreatedAt, &req.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if failureReason != nil {
		req.FailureReason = domain.FailureReason(*failureReason)
	}
	if resultStorageKey != nil {
		result := domain.ProcessingResult{PackageReference: *resultStorageKey}
		if frameCount != nil {
			result.FrameCount = *frameCount
		}
		req.Result = &result
	}
	return &req, nil
}
