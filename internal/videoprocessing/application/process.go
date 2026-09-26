package application

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	notificationdomain "video-processor/internal/notification/domain"
	"video-processor/internal/videoprocessing/domain"
)

var errCorruptedVideo = errors.New("corrupted video")

var failureMessages = map[domain.FailureReason]string{
	domain.FailureInvalidFormat: "The uploaded file has an unsupported format.",
	domain.FailureSizeExceeded:  "The uploaded file exceeds the allowed size or duration.",
	domain.FailureCorruptedFile: "The uploaded file appears to be corrupted and could not be processed.",
	domain.FailureInternalError: "An internal error occurred while processing your video.",
}

// Processor is the use case the worker adapter invokes for each queued
// message. It's a separate type from Service (rather than extra fields on
// it) so the HTTP adapter's wiring never needs to know about the
// Notification context. It has no dependency on Identity at all: the
// request's UserEmail is a value copied at creation time, not looked up
// here — see domain.NewProcessingRequest.
type Processor struct {
	repo      domain.Repository
	storage   domain.Storage
	extractor domain.FrameExtractor
	notifier  notificationdomain.Notifier
}

func NewProcessor(repo domain.Repository, storage domain.Storage, extractor domain.FrameExtractor, notifier notificationdomain.Notifier) *Processor {
	return &Processor{repo: repo, storage: storage, extractor: extractor, notifier: notifier}
}

// Process runs one ProcessingRequest end to end. A returned error means an
// infrastructure failure the caller should route to the dead-letter queue —
// a business failure (bad video content) is recorded on the aggregate and
// persisted, and Process returns a nil error, since it's a valid documented
// outcome rather than a message-processing failure
// (docs/technical-architecture.md §7). The returned Status reports that
// business outcome (StatusCompleted/StatusFailed) so callers — e.g. the
// worker's metrics — can distinguish it without a second query; on an
// infrastructure error, the zero Status ("") is returned and must be
// ignored in favor of the error.
func (p *Processor) Process(ctx context.Context, requestID string) (domain.Status, error) {
	id, err := uuid.Parse(requestID)
	if err != nil {
		return "", fmt.Errorf("invalid request id %q: %w", requestID, err)
	}

	req, err := p.repo.FindByID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("load request: %w", err)
	}

	if err := req.StartProcessing(); err != nil {
		return req.Status, nil
	}
	if err := p.repo.Update(ctx, req); err != nil {
		return "", fmt.Errorf("mark processing: %w", err)
	}

	result, procErr := p.extractAndPackage(ctx, req)
	if procErr != nil {
		reason := domain.FailureInternalError
		if errors.Is(procErr, errCorruptedVideo) {
			reason = domain.FailureCorruptedFile
		}
		if err := req.RecordFailure(reason); err != nil {
			return "", fmt.Errorf("record failure: %w", err)
		}
		if err := p.repo.Update(ctx, req); err != nil {
			return "", err
		}
		p.notifyBestEffort(ctx, req, failureMessages[reason])
		return req.Status, nil
	}

	if err := req.CompleteProcessing(*result); err != nil {
		return "", fmt.Errorf("complete processing: %w", err)
	}
	if err := p.repo.Update(ctx, req); err != nil {
		return "", err
	}
	p.notifyBestEffort(ctx, req, "")
	return req.Status, nil
}

// notifyBestEffort sends the outcome email after the aggregate's state is
// already durably persisted, so a notification failure never affects the
// request's status (docs/use-cases.md UC07 3a, docs/event-storming.md).
// reason is empty for a success notification.
func (p *Processor) notifyBestEffort(ctx context.Context, req *domain.ProcessingRequest, reason string) {
	var err error
	if reason == "" {
		err = p.notifier.NotifyCompleted(ctx, req.UserEmail, req.Metadata.OriginalName)
	} else {
		err = p.notifier.NotifyFailed(ctx, req.UserEmail, req.Metadata.OriginalName, reason)
	}
	if err != nil {
		log.Printf("notify request %s: %v", req.ID, err)
	}
}

func (p *Processor) extractAndPackage(ctx context.Context, req *domain.ProcessingRequest) (*domain.ProcessingResult, error) {
	workDir, err := os.MkdirTemp("", "processing-*")
	if err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	videoPath := filepath.Join(workDir, "input"+filepath.Ext(req.Metadata.OriginalName))
	if err := p.storage.DownloadVideo(ctx, req.VideoStorageKey, videoPath); err != nil {
		return nil, fmt.Errorf("download video: %w", err)
	}

	framesDir := filepath.Join(workDir, "frames")
	if err := os.Mkdir(framesDir, 0o755); err != nil {
		return nil, fmt.Errorf("create frames dir: %w", err)
	}

	frameCount, err := p.extractor.ExtractFrames(ctx, videoPath, framesDir)
	if err != nil || frameCount == 0 {
		return nil, fmt.Errorf("%w: %v", errCorruptedVideo, err)
	}

	zipPath := filepath.Join(workDir, "result.zip")
	if err := zipDirectory(framesDir, zipPath); err != nil {
		return nil, fmt.Errorf("create zip: %w", err)
	}

	resultKey := req.ID.String() + ".zip"
	if err := p.storage.UploadResult(ctx, resultKey, zipPath); err != nil {
		return nil, fmt.Errorf("upload result: %w", err)
	}

	return &domain.ProcessingResult{FrameCount: frameCount, PackageReference: resultKey}, nil
}

func zipDirectory(srcDir, destZip string) error {
	out, err := os.Create(destZip)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := addFileToZip(zw, filepath.Join(srcDir, entry.Name()), entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func addFileToZip(zw *zip.Writer, path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}
