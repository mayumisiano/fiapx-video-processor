package http

import (
	"errors"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"video-processor/internal/videoprocessing/application"
	"video-processor/internal/videoprocessing/domain"
)

type Handler struct {
	service *application.Service
}

func NewHandler(service *application.Service) *Handler {
	return &Handler{service: service}
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

func rejectionError(reason domain.FailureReason) *gin.H {
	messages := map[domain.FailureReason]string{
		domain.FailureInvalidFormat: "Unsupported video format. Accepted formats: mp4, mov, mkv, webm.",
		domain.FailureSizeExceeded:  "Video exceeds the maximum allowed size (500 MB) or duration (30 minutes).",
		domain.FailureCorruptedFile: "Could not read the video file. It may be corrupted.",
	}
	return &gin.H{"code": string(reason), "message": messages[reason]}
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

	userEmail := c.GetString("userEmail")

	results := make([]uploadResultItem, 0, len(form.File["videos"]))
	for _, fileHeader := range form.File["videos"] {
		results = append(results, h.uploadOne(c, userID, userEmail, fileHeader))
	}

	c.JSON(http.StatusCreated, gin.H{"results": results})
}

func (h *Handler) uploadOne(c *gin.Context, userID uuid.UUID, userEmail string, fileHeader *multipart.FileHeader) uploadResultItem {
	item := uploadResultItem{FileName: fileHeader.Filename}

	src, err := fileHeader.Open()
	if err != nil {
		item.Error = &gin.H{"code": "INTERNAL_ERROR", "message": "Could not read uploaded file"}
		return item
	}
	defer src.Close()

	outcome, err := h.service.UploadVideo(c.Request.Context(), userID, userEmail, fileHeader.Filename, src)
	if err != nil {
		item.Error = &gin.H{"code": "INTERNAL_ERROR", "message": "Could not process upload"}
		return item
	}

	if !outcome.Accepted {
		item.Error = rejectionError(outcome.Reason)
		return item
	}

	summary := toSummary(outcome.Request, false)
	item.Accepted = true
	item.Request = &summary
	return item
}

func (h *Handler) List(c *gin.Context) {
	userID, err := authenticatedUserID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Authorization header")
		return
	}

	requests, err := h.service.ListByUser(c.Request.Context(), userID)
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

func (h *Handler) Download(c *gin.Context) {
	userID, err := authenticatedUserID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Authorization header")
		return
	}

	requestID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errorResponse(c, http.StatusNotFound, "REQUEST_NOT_FOUND", "Processing request not found")
		return
	}

	url, expiresIn, err := h.service.GetDownloadURL(c.Request.Context(), userID, requestID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrRequestNotFound):
			errorResponse(c, http.StatusNotFound, "REQUEST_NOT_FOUND", "Processing request not found")
		case errors.Is(err, domain.ErrResultNotReady):
			errorResponse(c, http.StatusConflict, "RESULT_NOT_READY", "Result is not ready yet")
		case errors.Is(err, domain.ErrResultExpired):
			errorResponse(c, http.StatusGone, "RESULT_EXPIRED", "Result has passed the retention window")
		default:
			errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not generate download URL")
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"downloadUrl": url, "expiresIn": expiresIn})
}

func (h *Handler) Retry(c *gin.Context) {
	userID, err := authenticatedUserID(c)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Authorization header")
		return
	}

	requestID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		errorResponse(c, http.StatusNotFound, "REQUEST_NOT_FOUND", "Processing request not found")
		return
	}

	req, err := h.service.RetryVideo(c.Request.Context(), userID, requestID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrRequestNotFound):
			errorResponse(c, http.StatusNotFound, "REQUEST_NOT_FOUND", "Processing request not found")
		case errors.Is(err, domain.ErrNotFailed):
			errorResponse(c, http.StatusConflict, "NOT_FAILED", "Request is not currently failed")
		case errors.Is(err, domain.ErrRetriesExhausted):
			errorResponse(c, http.StatusConflict, "RETRIES_EXHAUSTED", "Maximum retry attempts reached")
		case errors.Is(err, domain.ErrOriginalVideoUnavailable):
			errorResponse(c, http.StatusGone, "ORIGINAL_VIDEO_UNAVAILABLE", "Original video is no longer available")
		default:
			errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not update processing request")
		}
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"request": toSummary(req, true)})
}
