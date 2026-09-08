package domain

import (
	"path/filepath"
	"strings"
)

// SniffFormat cross-checks a client-declared file extension against the
// file's magic bytes, so a renamed file (e.g. .exe renamed to .mp4) can't
// bypass format validation by extension alone (see docs/technical-architecture.md §8).
func SniffFormat(originalName string, header []byte) (format string, matchesMagicBytes bool) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(originalName), "."))
	if !AcceptedFormats[ext] {
		return ext, false
	}

	switch ext {
	case "mp4", "mov":
		return ext, isISOBaseMediaFile(header)
	case "mkv", "webm":
		return ext, isEBMLFile(header)
	default:
		return ext, false
	}
}

func isISOBaseMediaFile(header []byte) bool {
	return len(header) >= 8 && string(header[4:8]) == "ftyp"
}

func isEBMLFile(header []byte) bool {
	return len(header) >= 4 &&
		header[0] == 0x1A && header[1] == 0x45 && header[2] == 0xDF && header[3] == 0xA3
}
