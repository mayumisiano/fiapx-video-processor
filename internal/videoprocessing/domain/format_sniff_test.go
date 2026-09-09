package domain_test

import (
	"testing"

	"video-processor/internal/videoprocessing/domain"
)

func TestSniffFormat(t *testing.T) {
	isoHeader := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 0x00}
	ebmlHeader := []byte{0x1A, 0x45, 0xDF, 0xA3, 0x00}

	tests := []struct {
		name         string
		originalName string
		header       []byte
		wantFormat   string
		wantMatches  bool
	}{
		{
			name:         "mp4 with matching magic bytes",
			originalName: "clip.mp4",
			header:       isoHeader,
			wantFormat:   "mp4",
			wantMatches:  true,
		},
		{
			name:         "mkv with matching magic bytes",
			originalName: "clip.mkv",
			header:       ebmlHeader,
			wantFormat:   "mkv",
			wantMatches:  true,
		},
		{
			name:         "extension renamed to bypass validation",
			originalName: "malware.mp4",
			header:       []byte("not a real video file"),
			wantFormat:   "mp4",
			wantMatches:  false,
		},
		{
			name:         "extension and magic bytes mismatched families",
			originalName: "clip.mp4",
			header:       ebmlHeader,
			wantFormat:   "mp4",
			wantMatches:  false,
		},
		{
			name:         "unsupported extension",
			originalName: "clip.avi",
			header:       isoHeader,
			wantFormat:   "avi",
			wantMatches:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, matches := domain.SniffFormat(tt.originalName, tt.header)

			if format != tt.wantFormat {
				t.Errorf("format = %q, want %q", format, tt.wantFormat)
			}
			if matches != tt.wantMatches {
				t.Errorf("matches = %v, want %v", matches, tt.wantMatches)
			}
		})
	}
}
