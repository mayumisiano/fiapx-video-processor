package jwt

import (
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"

	"video-processor/internal/identity/domain"
)

const expiry = 24 * time.Hour

type claims struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	jwtlib.RegisteredClaims
}

// Issuer implements domain.TokenIssuer using HS256-signed JWTs.
type Issuer struct {
	secret []byte
}

func NewIssuer(secret string) *Issuer {
	return &Issuer{secret: []byte(secret)}
}

func (i *Issuer) Issue(user *domain.User) (string, error) {
	now := time.Now()
	c := claims{
		UserID: user.ID.String(),
		Email:  user.Email,
		RegisteredClaims: jwtlib.RegisteredClaims{
			IssuedAt:  jwtlib.NewNumericDate(now),
			ExpiresAt: jwtlib.NewNumericDate(now.Add(expiry)),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, c)
	return token.SignedString(i.secret)
}

func (i *Issuer) Parse(tokenString string) (*domain.TokenClaims, error) {
	c := &claims{}
	token, err := jwtlib.ParseWithClaims(tokenString, c, func(t *jwtlib.Token) (interface{}, error) {
		return i.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, domain.ErrInvalidToken
	}
	return &domain.TokenClaims{UserID: c.UserID, Email: c.Email}, nil
}
