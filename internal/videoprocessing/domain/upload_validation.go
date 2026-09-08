package domain

const (
	MaxSizeBytes       int64   = 500 * 1024 * 1024
	MaxDurationSeconds float64 = 30 * 60
)

var AcceptedFormats = map[string]bool{
	"mp4":  true,
	"mov":  true,
	"mkv":  true,
	"webm": true,
}

// ValidateUpload returns the first violated rule, in the order mandated by
// docs/domain-modeling.md §7.2/7.6: format, then size, then duration.
// Duration violations reuse SIZE_EXCEEDED — the documented failure-reason
// enum has no dedicated code for "too long", and it's the closest fit.
func ValidateUpload(metadata VideoMetadata) (reason FailureReason, ok bool) {
	if !AcceptedFormats[metadata.Format] {
		return FailureInvalidFormat, false
	}
	if metadata.SizeBytes > MaxSizeBytes {
		return FailureSizeExceeded, false
	}
	if metadata.DurationSeconds > MaxDurationSeconds {
		return FailureSizeExceeded, false
	}
	return "", true
}
