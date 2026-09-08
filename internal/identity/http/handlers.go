package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"video-processor/internal/identity/domain"
	identityjwt "video-processor/internal/identity/jwt"
)

type Handler struct {
	repo   domain.Repository
	issuer *identityjwt.Issuer
}

func NewHandler(repo domain.Repository, issuer *identityjwt.Issuer) *Handler {
	return &Handler{repo: repo, issuer: issuer}
}

func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.POST("/auth/register", h.Register)
	r.POST("/auth/login", h.Login)
}

func (h *Handler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		const prefix = "Bearer "
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, prefix) {
			errorResponse(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid Authorization header")
			c.Abort()
			return
		}

		claims, err := h.issuer.Parse(strings.TrimPrefix(header, prefix))
		if err != nil {
			errorResponse(c, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired token")
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("userEmail", claims.Email)
		c.Next()
	}
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

func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid email format or password below minimum length")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not process password")
		return
	}

	user := &domain.User{
		ID:           uuid.New(),
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
	}

	if err := h.repo.Create(c.Request.Context(), user); err != nil {
		if errors.Is(err, domain.ErrEmailAlreadyExists) {
			errorResponse(c, http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "Email already exists")
			return
		}
		errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not create user")
		return
	}

	token, err := h.issuer.Issue(user)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not issue token")
		return
	}

	c.JSON(http.StatusCreated, authResponse{
		User:  userResponse{ID: user.ID.String(), Name: user.Name, Email: user.Email},
		Token: token,
	})
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return
	}

	user, err := h.repo.FindByEmail(c.Request.Context(), req.Email)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		errorResponse(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
		return
	}

	token, err := h.issuer.Issue(user)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not issue token")
		return
	}

	c.JSON(http.StatusOK, authResponse{
		User:  userResponse{ID: user.ID.String(), Name: user.Name, Email: user.Email},
		Token: token,
	})
}
