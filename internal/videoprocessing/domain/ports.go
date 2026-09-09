package domain

import (
	"context"
	"time"
)

// Storage is the port for the object storage backing videos and results
// (implemented by internal/videoprocessing/storage against MinIO).
type Storage interface {
	UploadVideo(ctx context.Context, key, filePath, contentType string) error
	StatVideo(ctx context.Context, key string) error
	DownloadVideo(ctx context.Context, key, destPath string) error
	UploadResult(ctx context.Context, key, filePath string) error
	PresignedResultURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}

// Publisher is the port for enqueueing a request for processing
// (implemented by internal/videoprocessing/queue against RabbitMQ).
type Publisher interface {
	Publish(ctx context.Context, requestID string) error
}

// FrameExtractor is the port for inspecting and extracting frames from a
// video file (implemented by internal/videoprocessing/ffmpeg).
type FrameExtractor interface {
	ProbeDurationSeconds(ctx context.Context, filePath string) (float64, error)
	ExtractFrames(ctx context.Context, videoPath, outputDir string) (int, error)
}
