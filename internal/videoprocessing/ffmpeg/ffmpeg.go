package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// framesPerSecond controls how many frames are extracted per second of
// video. Not specified by the domain docs beyond "extract frames" — chosen
// as a reasonable default for the hackathon MVP.
const framesPerSecond = 1

// Extractor implements domain.FrameExtractor by shelling out to the
// ffmpeg/ffprobe binaries.
type Extractor struct{}

func NewExtractor() *Extractor {
	return &Extractor{}
}

func (e *Extractor) ProbeDurationSeconds(ctx context.Context, filePath string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(out.String()), 64)
	if err != nil {
		return 0, fmt.Errorf("parse ffprobe output: %w", err)
	}
	return duration, nil
}

func (e *Extractor) ExtractFrames(ctx context.Context, videoPath, outputDir string) (int, error) {
	pattern := filepath.Join(outputDir, "frame-%04d.jpg")
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=%d", framesPerSecond),
		pattern,
	)
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("ffmpeg: %w", err)
	}

	matches, err := filepath.Glob(filepath.Join(outputDir, "frame-*.jpg"))
	if err != nil {
		return 0, err
	}
	return len(matches), nil
}
