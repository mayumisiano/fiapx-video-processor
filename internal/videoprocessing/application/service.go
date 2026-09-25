package application

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/google/uuid"

	"video-processor/internal/videoprocessing/domain"
)

// UploadOutcome mirrors the per-file result POST /videos returns: either the
// created request, or the reason it was rejected before ever being created.
type UploadOutcome struct {
	Accepted bool
	Request  *domain.ProcessingRequest
	Reason   domain.FailureReason
}

// Service is the Video Processing context's application layer: it
// orchestrates the domain and its ports (Repository, Storage, Publisher,
// FrameExtractor), with no knowledge of HTTP or AMQP.
type Service struct {
	repo      domain.Repository
	storage   domain.Storage
	publisher domain.Publisher
	extractor domain.FrameExtractor
}

func NewService(repo domain.Repository, storage domain.Storage, publisher domain.Publisher, extractor domain.FrameExtractor) *Service {
	return &Service{repo: repo, storage: storage, publisher: publisher, extractor: extractor}
}

func (s *Service) UploadVideo(ctx context.Context, userID uuid.UUID, userEmail, fileName string, src io.Reader) (*UploadOutcome, error) {
	tempPath, sizeBytes, magicBytes, err := saveToTemp(src)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempPath)

	format, matches := domain.SniffFormat(fileName, magicBytes)
	if !matches {
		return &UploadOutcome{Reason: domain.FailureInvalidFormat}, nil
	}

	duration, err := s.extractor.ProbeDurationSeconds(ctx, tempPath)
	if err != nil {
		return &UploadOutcome{Reason: domain.FailureCorruptedFile}, nil
	}

	metadata := domain.VideoMetadata{
		OriginalName:    fileName,
		SizeBytes:       sizeBytes,
		Format:          format,
		DurationSeconds: duration,
	}

	if reason, ok := domain.ValidateUpload(metadata); !ok {
		return &UploadOutcome{Reason: reason}, nil
	}

	req := domain.NewProcessingRequest(userID, userEmail, metadata, "")
	req.VideoStorageKey = req.ID.String() + "/" + fileName

	if err := s.storage.UploadVideo(ctx, req.VideoStorageKey, tempPath, "video/"+format); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, req); err != nil {
		return nil, err
	}
	if err := s.publisher.Publish(ctx, req.ID.String()); err != nil {
		return nil, err
	}

	return &UploadOutcome{Accepted: true, Request: req}, nil
}

func saveToTemp(src io.Reader) (path string, size int64, magicBytes []byte, err error) {
	dst, err := os.CreateTemp("", "upload-*")
	if err != nil {
		return "", 0, nil, err
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		os.Remove(dst.Name())
		return "", 0, nil, err
	}

	header := make([]byte, 12)
	n, _ := dst.ReadAt(header, 0)

	return dst.Name(), written, header[:n], nil
}

func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID) ([]*domain.ProcessingRequest, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *Service) GetDownloadURL(ctx context.Context, userID, requestID uuid.UUID) (url string, expiresInSeconds int, err error) {
	req, err := s.loadOwned(ctx, requestID, userID)
	if err != nil {
		return "", 0, err
	}

	if req.Status != domain.StatusCompleted {
		return "", 0, domain.ErrResultNotReady
	}

	if time.Since(req.UpdatedAt) > domain.RetentionPeriod {
		return "", 0, domain.ErrResultExpired
	}

	url, err = s.storage.PresignedResultURL(ctx, req.Result.PackageReference, domain.DownloadURLExpiry)
	if err != nil {
		return "", 0, err
	}
	return url, int(domain.DownloadURLExpiry.Seconds()), nil
}

func (s *Service) RetryVideo(ctx context.Context, userID, requestID uuid.UUID) (*domain.ProcessingRequest, error) {
	req, err := s.loadOwned(ctx, requestID, userID)
	if err != nil {
		return nil, err
	}

	if req.Status != domain.StatusFailed {
		return nil, domain.ErrNotFailed
	}

	if err := s.storage.StatVideo(ctx, req.VideoStorageKey); err != nil {
		return nil, domain.ErrOriginalVideoUnavailable
	}

	if err := req.RetryProcessing(); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, req); err != nil {
		return nil, err
	}
	if err := s.publisher.Publish(ctx, req.ID.String()); err != nil {
		return nil, err
	}
	return req, nil
}

func (s *Service) loadOwned(ctx context.Context, requestID, userID uuid.UUID) (*domain.ProcessingRequest, error) {
	req, err := s.repo.FindByID(ctx, requestID)
	if err != nil || req.UserID != userID {
		return nil, domain.ErrRequestNotFound
	}
	return req, nil
}
