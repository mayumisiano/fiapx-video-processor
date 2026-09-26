package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"video-processor/internal/identity/application"
	"video-processor/internal/identity/domain"
)

type Handler struct {
	service *application.Service
}

func NewHandler(service *application.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r gin.IRouter, loginRateLimit gin.HandlerFunc) {
	r.POST("/auth/register", h.Register)
	r.POST("/auth/login", loginRateLimit, h.Login)
}

type registerRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type userResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type authResponse struct {
	User  userResponse `json:"user"`
	Token string       `json:"token"`
}

func errorResponse(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

func toAuthResponse(result *application.AuthResult) authResponse {
	return authResponse{
		User: userResponse{
			ID:    result.User.ID.String(),
			Name:  result.User.Name,
			Email: result.User.Email,
		},
		Token: result.Token,
	}
}

func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid email format or password below minimum length")
		return
	}

	result, err := h.service.Register(c.Request.Context(), req.Name, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, domain.ErrEmailAlreadyExists) {
			errorResponse(c, http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "Email already exists")
			return
		}
		errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not create user")
		return
	}

	c.JSON(http.StatusCreated, toAuthResponse(result))
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	result, err := h.service.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			errorResponse(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
			return
		}
		errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not authenticate")
		return
	}

	c.JSON(http.StatusOK, toAuthResponse(result))
}
