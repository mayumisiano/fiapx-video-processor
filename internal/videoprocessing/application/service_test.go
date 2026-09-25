package application_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"video-processor/internal/videoprocessing/application"
	"video-processor/internal/videoprocessing/domain"
)

// mp4Header carries a valid ISO-BMFF "ftyp" box so domain.SniffFormat
// accepts it as a genuine mp4, matching the ".mp4" extension under test.
var mp4Header = []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}

type fakeStorage struct {
	uploadVideoErr  error
	statVideoErr    error
	downloadErr     error
	uploadResultErr error
	presignedURL    string
	presignedURLErr error
}

func (s *fakeStorage) UploadVideo(ctx context.Context, key, filePath, contentType string) error {
	return s.uploadVideoErr
}
func (s *fakeStorage) StatVideo(ctx context.Context, key string) error { return s.statVideoErr }
func (s *fakeStorage) DownloadVideo(ctx context.Context, key, destPath string) error {
	return s.downloadErr
}
func (s *fakeStorage) UploadResult(ctx context.Context, key, filePath string) error {
	return s.uploadResultErr
}
func (s *fakeStorage) PresignedResultURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return s.presignedURL, s.presignedURLErr
}

type fakePublisher struct {
	published []string
	err       error
}

func (p *fakePublisher) Publish(ctx context.Context, requestID string) error {
	p.published = append(p.published, requestID)
	return p.err
}

type fakeExtractor struct {
	duration     float64
	err          error
	extractCount int
	extractErr   error
}

func (e *fakeExtractor) ProbeDurationSeconds(ctx context.Context, filePath string) (float64, error) {
	return e.duration, e.err
}
func (e *fakeExtractor) ExtractFrames(ctx context.Context, videoPath, outputDir string) (int, error) {
	return e.extractCount, e.extractErr
}

type fakeRepo struct {
	byID   map[uuid.UUID]*domain.ProcessingRequest
	create error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[uuid.UUID]*domain.ProcessingRequest{}}
}

func (r *fakeRepo) Create(ctx context.Context, req *domain.ProcessingRequest) error {
	if r.create != nil {
		return r.create
	}
	r.byID[req.ID] = req
	return nil
}
func (r *fakeRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.ProcessingRequest, error) {
	if req, ok := r.byID[id]; ok {
		return req, nil
	}
	return nil, domain.ErrRequestNotFound
}
func (r *fakeRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]*domain.ProcessingRequest, error) {
	return nil, nil
}
func (r *fakeRepo) Update(ctx context.Context, req *domain.ProcessingRequest) error {
	r.byID[req.ID] = req
	return nil
}

func newTestService(repo *fakeRepo, storage *fakeStorage, publisher *fakePublisher, extractor *fakeExtractor) *application.Service {
	return application.NewService(repo, storage, publisher, extractor)
}

func TestUploadVideo_ValidFile_IsAcceptedAndPublished(t *testing.T) {
	repo := newFakeRepo()
	publisher := &fakePublisher{}
	svc := newTestService(repo, &fakeStorage{}, publisher, &fakeExtractor{duration: 30})

	outcome, err := svc.UploadVideo(context.Background(), uuid.New(), "ada@example.com", "movie.mp4", bytes.NewReader(mp4Header))

	if err != nil {
		t.Fatalf("UploadVideo() error = %v, want nil", err)
	}
	if !outcome.Accepted {
		t.Fatalf("Accepted = false, reason = %q, want true", outcome.Reason)
	}
	if len(repo.byID) != 1 {
		t.Errorf("repo has %d requests, want 1", len(repo.byID))
	}
	if len(publisher.published) != 1 {
		t.Errorf("publisher received %d messages, want 1", len(publisher.published))
	}
}

func TestUploadVideo_ExtensionDoesNotMatchMagicBytes_RejectedWithoutSideEffects(t *testing.T) {
	repo := newFakeRepo()
	publisher := &fakePublisher{}
	svc := newTestService(repo, &fakeStorage{}, publisher, &fakeExtractor{duration: 30})

	outcome, err := svc.UploadVideo(context.Background(), uuid.New(), "ada@example.com", "movie.mp4", bytes.NewReader([]byte("not a real video")))

	if err != nil {
		t.Fatalf("UploadVideo() error = %v, want nil", err)
	}
	if outcome.Accepted {
		t.Fatalf("Accepted = true, want false")
	}
	if outcome.Reason != domain.FailureInvalidFormat {
		t.Errorf("Reason = %q, want %q", outcome.Reason, domain.FailureInvalidFormat)
	}
	if len(repo.byID) != 0 || len(publisher.published) != 0 {
		t.Errorf("rejected upload must not create or publish anything")
	}
}

func TestUploadVideo_ExtractorFailsToProbe_RejectedAsCorrupted(t *testing.T) {
	svc := newTestService(newFakeRepo(), &fakeStorage{}, &fakePublisher{}, &fakeExtractor{err: errors.New("ffprobe: exit status 1")})

	outcome, err := svc.UploadVideo(context.Background(), uuid.New(), "ada@example.com", "movie.mp4", bytes.NewReader(mp4Header))

	if err != nil {
		t.Fatalf("UploadVideo() error = %v, want nil", err)
	}
	if outcome.Accepted || outcome.Reason != domain.FailureCorruptedFile {
		t.Errorf("outcome = %+v, want rejected with %q", outcome, domain.FailureCorruptedFile)
	}
}

func TestUploadVideo_DurationExceedsLimit_RejectedAsSizeExceeded(t *testing.T) {
	svc := newTestService(newFakeRepo(), &fakeStorage{}, &fakePublisher{}, &fakeExtractor{duration: domain.MaxDurationSeconds + 1})

	outcome, err := svc.UploadVideo(context.Background(), uuid.New(), "ada@example.com", "movie.mp4", bytes.NewReader(mp4Header))

	if err != nil {
		t.Fatalf("UploadVideo() error = %v, want nil", err)
	}
	if outcome.Accepted || outcome.Reason != domain.FailureSizeExceeded {
		t.Errorf("outcome = %+v, want rejected with %q", outcome, domain.FailureSizeExceeded)
	}
}

func completedRequest(userID uuid.UUID, updatedAt time.Time) *domain.ProcessingRequest {
	req := domain.NewProcessingRequest(userID, "ada@example.com", domain.VideoMetadata{OriginalName: "movie.mp4", Format: "mp4"}, "videos/movie.mp4")
	if err := req.StartProcessing(); err != nil {
		panic(err)
	}
	if err := req.CompleteProcessing(domain.ProcessingResult{FrameCount: 10, PackageReference: "results/x.zip"}); err != nil {
		panic(err)
	}
	req.UpdatedAt = updatedAt
	return req
}

func TestGetDownloadURL_CompletedWithinRetention_ReturnsPresignedURL(t *testing.T) {
	userID := uuid.New()
	req := completedRequest(userID, time.Now())
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	svc := newTestService(repo, &fakeStorage{presignedURL: "https://minio.local/signed"}, &fakePublisher{}, &fakeExtractor{})

	url, expiresIn, err := svc.GetDownloadURL(context.Background(), userID, req.ID)

	if err != nil {
		t.Fatalf("GetDownloadURL() error = %v, want nil", err)
	}
	if url != "https://minio.local/signed" {
		t.Errorf("url = %q, want the presigned URL", url)
	}
	if expiresIn != int(domain.DownloadURLExpiry.Seconds()) {
		t.Errorf("expiresIn = %d, want %d", expiresIn, int(domain.DownloadURLExpiry.Seconds()))
	}
}

func TestGetDownloadURL_NotCompleted_ReturnsErrResultNotReady(t *testing.T) {
	userID := uuid.New()
	req := domain.NewProcessingRequest(userID, "ada@example.com", domain.VideoMetadata{Format: "mp4"}, "videos/movie.mp4")
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	svc := newTestService(repo, &fakeStorage{}, &fakePublisher{}, &fakeExtractor{})

	_, _, err := svc.GetDownloadURL(context.Background(), userID, req.ID)

	if !errors.Is(err, domain.ErrResultNotReady) {
		t.Errorf("GetDownloadURL() error = %v, want %v", err, domain.ErrResultNotReady)
	}
}

func TestGetDownloadURL_PastRetentionWindow_ReturnsErrResultExpired(t *testing.T) {
	userID := uuid.New()
	req := completedRequest(userID, time.Now().Add(-domain.RetentionPeriod-time.Hour))
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	svc := newTestService(repo, &fakeStorage{}, &fakePublisher{}, &fakeExtractor{})

	_, _, err := svc.GetDownloadURL(context.Background(), userID, req.ID)

	if !errors.Is(err, domain.ErrResultExpired) {
		t.Errorf("GetDownloadURL() error = %v, want %v", err, domain.ErrResultExpired)
	}
}

func TestGetDownloadURL_DifferentOwner_ReturnsErrRequestNotFound(t *testing.T) {
	req := completedRequest(uuid.New(), time.Now())
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	svc := newTestService(repo, &fakeStorage{}, &fakePublisher{}, &fakeExtractor{})

	_, _, err := svc.GetDownloadURL(context.Background(), uuid.New(), req.ID)

	if !errors.Is(err, domain.ErrRequestNotFound) {
		t.Errorf("GetDownloadURL() error = %v, want %v", err, domain.ErrRequestNotFound)
	}
}

func failedRequest(userID uuid.UUID) *domain.ProcessingRequest {
	req := domain.NewProcessingRequest(userID, "ada@example.com", domain.VideoMetadata{OriginalName: "movie.mp4", Format: "mp4"}, "videos/movie.mp4")
	if err := req.StartProcessing(); err != nil {
		panic(err)
	}
	if err := req.RecordFailure(domain.FailureInternalError); err != nil {
		panic(err)
	}
	return req
}

func TestRetryVideo_FailedWithVideoAvailable_ResetsAndPublishes(t *testing.T) {
	userID := uuid.New()
	req := failedRequest(userID)
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	publisher := &fakePublisher{}
	svc := newTestService(repo, &fakeStorage{}, publisher, &fakeExtractor{})

	updated, err := svc.RetryVideo(context.Background(), userID, req.ID)

	if err != nil {
		t.Fatalf("RetryVideo() error = %v, want nil", err)
	}
	if updated.Status != domain.StatusPending {
		t.Errorf("Status = %q, want %q", updated.Status, domain.StatusPending)
	}
	if len(publisher.published) != 1 {
		t.Errorf("publisher received %d messages, want 1", len(publisher.published))
	}
}

func TestRetryVideo_NotFailed_ReturnsErrNotFailed(t *testing.T) {
	userID := uuid.New()
	req := domain.NewProcessingRequest(userID, "ada@example.com", domain.VideoMetadata{Format: "mp4"}, "videos/movie.mp4")
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	svc := newTestService(repo, &fakeStorage{}, &fakePublisher{}, &fakeExtractor{})

	_, err := svc.RetryVideo(context.Background(), userID, req.ID)

	if !errors.Is(err, domain.ErrNotFailed) {
		t.Errorf("RetryVideo() error = %v, want %v", err, domain.ErrNotFailed)
	}
}

func TestRetryVideo_OriginalVideoMissing_ReturnsErrOriginalVideoUnavailable(t *testing.T) {
	userID := uuid.New()
	req := failedRequest(userID)
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	svc := newTestService(repo, &fakeStorage{statVideoErr: errors.New("not found")}, &fakePublisher{}, &fakeExtractor{})

	_, err := svc.RetryVideo(context.Background(), userID, req.ID)

	if !errors.Is(err, domain.ErrOriginalVideoUnavailable) {
		t.Errorf("RetryVideo() error = %v, want %v", err, domain.ErrOriginalVideoUnavailable)
	}
}

func TestRetryVideo_RetriesExhausted_PropagatesDomainError(t *testing.T) {
	userID := uuid.New()
	req := failedRequest(userID)
	req.Attempts = domain.MaxAttempts
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	svc := newTestService(repo, &fakeStorage{}, &fakePublisher{}, &fakeExtractor{})

	_, err := svc.RetryVideo(context.Background(), userID, req.ID)

	if !errors.Is(err, domain.ErrRetriesExhausted) {
		t.Errorf("RetryVideo() error = %v, want %v", err, domain.ErrRetriesExhausted)
	}
}
