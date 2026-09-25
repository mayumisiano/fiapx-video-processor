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
	ErrInvalidTransition        = errors.New("invalid state transition")
	ErrRetriesExhausted         = errors.New("retries exhausted")
	ErrRequestNotFound          = errors.New("processing request not found")
	ErrNotOwner                 = errors.New("processing request does not belong to this user")
	ErrResultNotReady           = errors.New("result is not ready yet")
	ErrResultExpired            = errors.New("result has passed the retention window")
	ErrNotFailed                = errors.New("request is not currently failed")
	ErrOriginalVideoUnavailable = errors.New("original video is no longer available")
)

// RetentionPeriod is how long a completed result stays downloadable
// (docs/domain-modeling.md §7.3, also enforced by a MinIO lifecycle policy).
const RetentionPeriod = 30 * 24 * time.Hour

// DownloadURLExpiry is how long a presigned download URL stays valid.
const DownloadURLExpiry = 5 * time.Minute

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
	UserEmail       string
	Metadata        VideoMetadata
	VideoStorageKey string
	Status          Status
	FailureReason   FailureReason
	Attempts        int
	Result          *ProcessingResult
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewProcessingRequest takes userEmail as a value copied from the trusted
// JWT claim at request time (Identity → Video Processing is a Conformist
// relationship — see docs/bounded-contexts.md). Video Processing owns its
// own database and never queries Identity's afterwards, including from the
// worker when it sends the outcome notification.
func NewProcessingRequest(userID uuid.UUID, userEmail string, metadata VideoMetadata, videoStorageKey string) *ProcessingRequest {
	now := time.Now()
	return &ProcessingRequest{
		ID:              uuid.New(),
		UserID:          userID,
		UserEmail:       userEmail,
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
