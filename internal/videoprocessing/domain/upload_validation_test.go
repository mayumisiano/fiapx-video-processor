package domain_test

import (
	"testing"

	"video-processor/internal/videoprocessing/domain"
)

func TestValidateUpload(t *testing.T) {
	tests := []struct {
		name       string
		metadata   domain.VideoMetadata
		wantReason domain.FailureReason
		wantOK     bool
	}{
		{
			name: "accepted format within limits",
			metadata: domain.VideoMetadata{
				Format: "mp4", SizeBytes: 1024, DurationSeconds: 60,
			},
			wantOK: true,
		},
		{
			name: "unsupported format",
			metadata: domain.VideoMetadata{
				Format: "avi", SizeBytes: 1024, DurationSeconds: 60,
			},
			wantReason: domain.FailureInvalidFormat,
		},
		{
			name: "size over limit",
			metadata: domain.VideoMetadata{
				Format: "mp4", SizeBytes: domain.MaxSizeBytes + 1, DurationSeconds: 60,
			},
			wantReason: domain.FailureSizeExceeded,
		},
		{
			name: "duration over limit",
			metadata: domain.VideoMetadata{
				Format: "mp4", SizeBytes: 1024, DurationSeconds: domain.MaxDurationSeconds + 1,
			},
			wantReason: domain.FailureSizeExceeded,
		},
		{
			name: "format checked before size",
			metadata: domain.VideoMetadata{
				Format: "avi", SizeBytes: domain.MaxSizeBytes + 1, DurationSeconds: 60,
			},
			wantReason: domain.FailureInvalidFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, ok := domain.ValidateUpload(tt.metadata)

			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}
