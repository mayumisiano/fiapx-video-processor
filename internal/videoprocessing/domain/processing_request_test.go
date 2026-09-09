package domain_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"video-processor/internal/videoprocessing/domain"
)

func newTestRequest() *domain.ProcessingRequest {
	return domain.NewProcessingRequest(uuid.New(), domain.VideoMetadata{
		OriginalName: "movie.mp4",
		SizeBytes:    1024,
		Format:       "mp4",
	}, "videos/movie.mp4")
}

func TestNewProcessingRequest_StartsPendingWithOneAttempt(t *testing.T) {
	req := newTestRequest()

	if req.Status != domain.StatusPending {
		t.Errorf("Status = %q, want %q", req.Status, domain.StatusPending)
	}
	if req.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", req.Attempts)
	}
}

func TestStartProcessing_FromPending_Succeeds(t *testing.T) {
	req := newTestRequest()

	if err := req.StartProcessing(); err != nil {
		t.Fatalf("StartProcessing() error = %v, want nil", err)
	}
	if req.Status != domain.StatusProcessing {
		t.Errorf("Status = %q, want %q", req.Status, domain.StatusProcessing)
	}
}

func TestStartProcessing_FromNonPending_Rejected(t *testing.T) {
	req := newTestRequest()
	mustStart(t, req)

	err := req.StartProcessing()

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("StartProcessing() error = %v, want %v", err, domain.ErrInvalidTransition)
	}
}

func TestCompleteProcessing_FromProcessing_Succeeds(t *testing.T) {
	req := newTestRequest()
	mustStart(t, req)
	result := domain.ProcessingResult{FrameCount: 42, PackageReference: "results/x.zip"}

	if err := req.CompleteProcessing(result); err != nil {
		t.Fatalf("CompleteProcessing() error = %v, want nil", err)
	}
	if req.Status != domain.StatusCompleted {
		t.Errorf("Status = %q, want %q", req.Status, domain.StatusCompleted)
	}
	if req.Result == nil || *req.Result != result {
		t.Errorf("Result = %+v, want %+v", req.Result, result)
	}
}

func TestCompleteProcessing_FromPending_Rejected(t *testing.T) {
	req := newTestRequest()

	err := req.CompleteProcessing(domain.ProcessingResult{})

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("CompleteProcessing() error = %v, want %v", err, domain.ErrInvalidTransition)
	}
}

func TestRecordFailure_FromProcessing_Succeeds(t *testing.T) {
	req := newTestRequest()
	mustStart(t, req)

	if err := req.RecordFailure(domain.FailureCorruptedFile); err != nil {
		t.Fatalf("RecordFailure() error = %v, want nil", err)
	}
	if req.Status != domain.StatusFailed {
		t.Errorf("Status = %q, want %q", req.Status, domain.StatusFailed)
	}
	if req.FailureReason != domain.FailureCorruptedFile {
		t.Errorf("FailureReason = %q, want %q", req.FailureReason, domain.FailureCorruptedFile)
	}
}

func TestRetryProcessing_FromFailed_IncrementsAttemptsAndResetsToPending(t *testing.T) {
	req := newTestRequest()
	mustStart(t, req)
	mustFail(t, req)

	if err := req.RetryProcessing(); err != nil {
		t.Fatalf("RetryProcessing() error = %v, want nil", err)
	}
	if req.Status != domain.StatusPending {
		t.Errorf("Status = %q, want %q", req.Status, domain.StatusPending)
	}
	if req.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", req.Attempts)
	}
	if req.FailureReason != "" {
		t.Errorf("FailureReason = %q, want empty", req.FailureReason)
	}
}

func TestRetryProcessing_FromNonFailed_Rejected(t *testing.T) {
	req := newTestRequest()

	err := req.RetryProcessing()

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("RetryProcessing() error = %v, want %v", err, domain.ErrInvalidTransition)
	}
}

func TestRetryProcessing_AtMaxAttempts_ExhaustsRetries(t *testing.T) {
	req := newTestRequest()

	for req.Attempts < domain.MaxAttempts {
		mustStart(t, req)
		mustFail(t, req)
		if err := req.RetryProcessing(); err != nil {
			t.Fatalf("RetryProcessing() error = %v, want nil while under MaxAttempts", err)
		}
	}
	mustStart(t, req)
	mustFail(t, req)

	err := req.RetryProcessing()

	if !errors.Is(err, domain.ErrRetriesExhausted) {
		t.Errorf("RetryProcessing() error = %v, want %v", err, domain.ErrRetriesExhausted)
	}
}

func mustStart(t *testing.T, req *domain.ProcessingRequest) {
	t.Helper()
	if err := req.StartProcessing(); err != nil {
		t.Fatalf("setup StartProcessing() error = %v", err)
	}
}

func mustFail(t *testing.T, req *domain.ProcessingRequest) {
	t.Helper()
	if err := req.RecordFailure(domain.FailureInternalError); err != nil {
		t.Fatalf("setup RecordFailure() error = %v", err)
	}
}
