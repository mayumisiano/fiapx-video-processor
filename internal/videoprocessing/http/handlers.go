package http

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"video-processor/internal/videoprocessing/domain"
	"video-processor/internal/videoprocessing/ffmpeg"
	"video-processor/internal/videoprocessing/queue"
	"video-processor/internal/videoprocessing/storage"
)

// retentionPeriod is how long a completed result stays downloadable, per
// docs/domain-modeling.md §7.3 (also enforced by a MinIO lifecycle policy).
const retentionPeriod = 30 * 24 * time.Hour

const downloadURLExpiry = 5 * time.Minute

type Handler struct {
	repo      domain.Repository
	storage   *storage.Client
	publisher *queue.Publisher
}

func NewHandler(repo domain.Repository, storageClient *storage.Client, publisher *queue.Publisher) *Handler {
	return &Handler{repo: repo, storage: storageClient, publisher: publisher}
}

func (h *Handler) RegisterRoutes(r gin.IRouter, requireAuth gin.HandlerFunc) {
	protected := r.Group("/videos", requireAuth)
	protected.POST("", h.Upload)
	protected.GET("", h.List)
	protected.GET("/:id/download", h.Download)
	protected.POST("/:id/retry", h.Retry)
}

func errorResponse(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

func authenticatedUserID(c *gin.Context) (uuid.UUID, error) {
	return uuid.Parse(c.GetString("userID"))
}

type requestSummary struct {
	ID            string `json:"id"`
	FileName      string `json:"fileName"`
	Status        string `json:"status"`
	FailureReason string `json:"failureReason,omitempty"`
	Attempts      int    `json:"attempts"`
	FrameCount    *int   `json:"frameCount,omitempty"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
}

func toSummary(r *domain.ProcessingRequest, includeUpdatedAt bool) requestSummary {
	s := requestSummary{
		ID:            r.ID.String(),
		FileName:      r.Metadata.OriginalName,
		Status:        string(r.Status),
		FailureReason: string(r.FailureReason),
		Attempts:      r.Attempts,
		CreatedAt:     r.CreatedAt.UTC().Format(time.RFC3339),
	}
	if r.Result != nil {
		s.FrameCount = &r.Result.FrameCount
	}
	if includeUpdatedAt {
		s.UpdatedAt = r.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return s
}

type uploadResultItem struct {
	FileName string          `json:"fileName"`
	Accepted bool            `json:"accepted"`
	Request  *requestSummary `json:"request,omitempty"`
	Error    *gin.H          `json:"error,omitempty"`
}

func (h *Handler) Upload(c *gin.Context) {
	userID, err := authenticatedUserID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Authorization header")
		return
	}

	form, err := c.MultipartForm()
	if err != nil || len(form.File["videos"]) == 0 {
		errorResponse(c, http.StatusBadRequest, "NO_FILES_PROVIDED", "No file included in the request")
		return
	}

	results := make([]uploadResultItem, 0, len(form.File["videos"]))
	for _, fileHeader := range form.File["videos"] {
		results = append(results, h.acceptFile(c, userID, fileHeader))
	}

	c.JSON(http.StatusCreated, gin.H{"results": results})
}

func (h *Handler) acceptFile(c *gin.Context, userID uuid.UUID, fileHeader *multipart.FileHeader) uploadResultItem {
	item := uploadResultItem{FileName: fileHeader.Filename}

	tempPath, sizeBytes, magicBytes, err := saveToTemp(fileHeader)
	if err != nil {
		item.Error = &gin.H{"code": "INTERNAL_ERROR", "message": "Could not read uploaded file"}
		return item
	}
	defer os.Remove(tempPath)

	format, matches := domain.SniffFormat(fileHeader.Filename, magicBytes)
	if !matches {
		item.Error = rejectionError(domain.FailureInvalidFormat)
		return item
	}

	duration, err := ffmpeg.ProbeDurationSeconds(c.Request.Context(), tempPath)
	if err != nil {
		item.Error = rejectionError(domain.FailureCorruptedFile)
		return item
	}

	metadata := domain.VideoMetadata{
		OriginalName:    fileHeader.Filename,
		SizeBytes:       sizeBytes,
		Format:          format,
		DurationSeconds: duration,
	}

	if reason, ok := domain.ValidateUpload(metadata); !ok {
		item.Error = rejectionError(reason)
		return item
	}

	req := domain.NewProcessingRequest(userID, metadata, "")
	videoStorageKey := req.ID.String() + "/" + fileHeader.Filename
	req.VideoStorageKey = videoStorageKey

	if err := h.storage.UploadVideo(c.Request.Context(), videoStorageKey, tempPath, "video/"+format); err != nil {
		item.Error = &gin.H{"code": "INTERNAL_ERROR", "message": "Could not store video"}
		return item
	}

	if err := h.repo.Create(c.Request.Context(), req); err != nil {
		item.Error = &gin.H{"code": "INTERNAL_ERROR", "message": "Could not create processing request"}
		return item
	}

	if err := h.publisher.Publish(c.Request.Context(), req.ID.String()); err != nil {
		item.Error = &gin.H{"code": "INTERNAL_ERROR", "message": "Could not enqueue processing request"}
		return item
	}

	summary := toSummary(req, false)
	item.Accepted = true
	item.Request = &summary
	return item
}

func saveToTemp(fileHeader *multipart.FileHeader) (path string, size int64, magicBytes []byte, err error) {
	src, err := fileHeader.Open()
	if err != nil {
		return "", 0, nil, err
	}
	defer src.Close()

	dst, err := os.CreateTemp("", "upload-*")
	if err != nil {
		return "", 0, nil, err
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		os.Remove(dst.Name())
		return "", 0, nil, err
	}

	header := make([]byte, 12)
	n, _ := dst.ReadAt(header, 0)

	return dst.Name(), written, header[:n], nil
}

func rejectionError(reason domain.FailureReason) *gin.H {
	messages := map[domain.FailureReason]string{
		domain.FailureInvalidFormat: "Unsupported video format. Accepted formats: mp4, mov, mkv, webm.",
		domain.FailureSizeExceeded:  "Video exceeds the maximum allowed size (500 MB) or duration (30 minutes).",
		domain.FailureCorruptedFile: "Could not read the video file. It may be corrupted.",
	}
	return &gin.H{"code": string(reason), "message": messages[reason]}
}

func (h *Handler) List(c *gin.Context) {
	userID, err := authenticatedUserID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Authorization header")
		return
	}

	requests, err := h.repo.ListByUser(c.Request.Context(), userID)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not list processing requests")
		return
	}

	summaries := make([]requestSummary, 0, len(requests))
	for _, r := range requests {
		summaries = append(summaries, toSummary(r, true))
	}
	c.JSON(http.StatusOK, gin.H{"requests": summaries})
}

func (h *Handler) loadOwnedRequest(c *gin.Context, userID uuid.UUID) (*domain.ProcessingRequest, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errorResponse(c, http.StatusNotFound, "REQUEST_NOT_FOUND", "Processing request not found")
		return nil, false
	}

	req, err := h.repo.FindByID(c.Request.Context(), id)
	if err != nil || req.UserID != userID {
		errorResponse(c, http.StatusNotFound, "REQUEST_NOT_FOUND", "Processing request not found")
		return nil, false
	}
	return req, true
}

func (h *Handler) Download(c *gin.Context) {
	userID, err := authenticatedUserID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Authorization header")
		return
	}

	req, ok := h.loadOwnedRequest(c, userID)
	if !ok {
		return
	}

	if req.Status != domain.StatusCompleted {
		errorResponse(c, http.StatusConflict, "RESULT_NOT_READY", "Result is not ready yet")
		return
	}

	if time.Since(req.UpdatedAt) > retentionPeriod {
		errorResponse(c, http.StatusGone, "RESULT_EXPIRED", "Result has passed the retention window")
		return
	}

	url, err := h.storage.PresignedResultURL(c.Request.Context(), req.Result.PackageReference, downloadURLExpiry)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate download URL")
		return
	}

	c.JSON(http.StatusOK, gin.H{"downloadUrl": url, "expiresIn": int(downloadURLExpiry.Seconds())})
}

func (h *Handler) Retry(c *gin.Context) {
	userID, err := authenticatedUserID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Authorization header")
		return
	}

	req, ok := h.loadOwnedRequest(c, userID)
	if !ok {
		return
	}

	if req.Status != domain.StatusFailed {
		errorResponse(c, http.StatusConflict, "NOT_FAILED", "Request is not currently failed")
		return
	}

	if err := h.storage.StatVideo(c.Request.Context(), req.VideoStorageKey); err != nil {
		errorResponse(c, http.StatusGone, "ORIGINAL_VIDEO_UNAVAILABLE", "Original video is no longer available")
		return
	}

	if err := req.RetryProcessing(); err != nil {
		if errors.Is(err, domain.ErrRetriesExhausted) {
			errorResponse(c, http.StatusConflict, "RETRIES_EXHAUSTED", "Maximum retry attempts reached")
			return
		}
		errorResponse(c, http.StatusConflict, "NOT_FAILED", "Request is not currently failed")
		return
	}

	if err := h.repo.Update(c.Request.Context(), req); err != nil {
		errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update processing request")
		return
	}

	if err := h.publisher.Publish(c.Request.Context(), req.ID.String()); err != nil {
		errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not enqueue processing request")
		return
	}

	summary := toSummary(req, true)
	c.JSON(http.StatusAccepted, gin.H{"request": summary})
}
