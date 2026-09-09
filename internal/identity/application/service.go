package application

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"video-processor/internal/identity/domain"
)

type AuthResult struct {
	User  *domain.User
	Token string
}

// Service is the Identity context's application layer: it orchestrates the
// domain and its ports (Repository, TokenIssuer), with no knowledge of how
// it's invoked (HTTP, CLI, tests, ...).
type Service struct {
	repo   domain.Repository
	issuer domain.TokenIssuer
}

func NewService(repo domain.Repository, issuer domain.TokenIssuer) *Service {
	return &Service{repo: repo, issuer: issuer}
}

func (s *Service) Register(ctx context.Context, name, email, password string) (*AuthResult, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		ID:           uuid.New(),
		Name:         name,
		Email:        email,
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return s.issueFor(user)
}

func (s *Service) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	return s.issueFor(user)
}

func (s *Service) Authenticate(tokenString string) (*domain.TokenClaims, error) {
	return s.issuer.Parse(tokenString)
}

func (s *Service) issueFor(user *domain.User) (*AuthResult, error) {
	token, err := s.issuer.Issue(user)
	if err != nil {
		return nil, err
	}
	return &AuthResult{User: user, Token: token}, nil
}
