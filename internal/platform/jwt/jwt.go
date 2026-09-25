// Package jwt verifies HS256 session tokens using a shared signing secret.
// It has no knowledge of Identity or any other bounded context — any
// service holding JWT_SECRET can verify a token issued by
// internal/identity/jwt without calling identity-api over the network.
// This is the trust boundary shared across services: a cryptographic
// secret, not a database.
package jwt

import (
	"errors"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	UserID string
	Email  string
}

type tokenClaims struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	jwtlib.RegisteredClaims
}

func Verify(secret, tokenString string) (*Claims, error) {
	c := &tokenClaims{}
	token, err := jwtlib.ParseWithClaims(tokenString, c, func(t *jwtlib.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return &Claims{UserID: c.UserID, Email: c.Email}, nil
}
