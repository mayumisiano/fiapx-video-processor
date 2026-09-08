package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending    Status = "PENDING"
	StatusProcessing Status = "PROCESSING"
	StatusCompleted  Status = "COMPLETED"
	StatusFailed     Status = "FAILED"
)

type FailureReason string

const (
	FailureInvalidFormat FailureReason = "INVALID_FORMAT"
	FailureSizeExceeded  FailureReason = "SIZE_EXCEEDED"
	FailureCorruptedFile FailureReason = "CORRUPTED_FILE"
	FailureInternalError FailureReason = "INTERNAL_ERROR"
)

const MaxAttempts = 3

var (
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrRetriesExhausted  = errors.New("retries exhausted")
	ErrRequestNotFound   = errors.New("processing request not found")
	ErrNotOwner          = errors.New("processing request does not belong to this user")
)

type VideoMetadata struct {
	OriginalName    string
	SizeBytes       int64
	Format          string
	DurationSeconds float64
}

type ProcessingResult struct {
	FrameCount       int
	PackageReference string
}

type ProcessingRequest struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	Metadata        VideoMetadata
	VideoStorageKey string
	Status          Status
	FailureReason   FailureReason
	Attempts        int
	Result          *ProcessingResult
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewProcessingRequest(userID uuid.UUID, metadata VideoMetadata, videoStorageKey string) *ProcessingRequest {
	now := time.Now()
	return &ProcessingRequest{
		ID:              uuid.New(),
		UserID:          userID,
		Metadata:        metadata,
		VideoStorageKey: videoStorageKey,
		Status:          StatusPending,
		Attempts:        1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func (r *ProcessingRequest) StartProcessing() error {
	if r.Status != StatusPending {
		return ErrInvalidTransition
	}
	r.Status = StatusProcessing
	r.UpdatedAt = time.Now()
	return nil
}

func (r *ProcessingRequest) CompleteProcessing(result ProcessingResult) error {
	if r.Status != StatusProcessing {
		return ErrInvalidTransition
	}
	r.Status = StatusCompleted
	r.Result = &result
	r.UpdatedAt = time.Now()
	return nil
}

func (r *ProcessingRequest) RecordFailure(reason FailureReason) error {
	if r.Status != StatusProcessing {
		return ErrInvalidTransition
	}
	r.Status = StatusFailed
	r.FailureReason = reason
	r.UpdatedAt = time.Now()
	return nil
}

func (r *ProcessingRequest) RetryProcessing() error {
	if r.Status != StatusFailed {
		return ErrInvalidTransition
	}
	if r.Attempts >= MaxAttempts {
		return ErrRetriesExhausted
	}
	r.Attempts++
	r.Status = StatusPending
	r.FailureReason = ""
	r.UpdatedAt = time.Now()
	return nil
}

type Repository interface {
	Create(ctx context.Context, request *ProcessingRequest) error
	FindByID(ctx context.Context, id uuid.UUID) (*ProcessingRequest, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*ProcessingRequest, error)
	Update(ctx context.Context, request *ProcessingRequest) error
}
