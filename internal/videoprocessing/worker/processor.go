package worker

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"video-processor/internal/videoprocessing/domain"
	"video-processor/internal/videoprocessing/ffmpeg"
	"video-processor/internal/videoprocessing/storage"
)

var errCorruptedVideo = errors.New("corrupted video")

type Processor struct {
	repo    domain.Repository
	storage *storage.Client
}

func NewProcessor(repo domain.Repository, storageClient *storage.Client) *Processor {
	return &Processor{repo: repo, storage: storageClient}
}

// Process runs one ProcessingRequest end to end. A returned error means an
// infrastructure failure the caller should route to the dead-letter queue —
// a business failure (bad video content) is recorded on the aggregate and
// persisted, and Process returns nil, since it's a valid documented outcome
// rather than a message-processing failure (docs/technical-architecture.md §7).
func (p *Processor) Process(ctx context.Context, requestID string) error {
	id, err := uuid.Parse(requestID)
	if err != nil {
		return fmt.Errorf("invalid request id %q: %w", requestID, err)
	}

	req, err := p.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("load request: %w", err)
	}

	if err := req.StartProcessing(); err != nil {
		return nil
	}
	if err := p.repo.Update(ctx, req); err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}

	result, procErr := p.extractAndPackage(ctx, req)
	if procErr != nil {
		reason := domain.FailureInternalError
		if errors.Is(procErr, errCorruptedVideo) {
			reason = domain.FailureCorruptedFile
		}
		if err := req.RecordFailure(reason); err != nil {
			return fmt.Errorf("record failure: %w", err)
		}
		return p.repo.Update(ctx, req)
	}

	if err := req.CompleteProcessing(*result); err != nil {
		return fmt.Errorf("complete processing: %w", err)
	}
	return p.repo.Update(ctx, req)
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

	frameCount, err := ffmpeg.ExtractFrames(ctx, videoPath, framesDir)
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
