package apiserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type ctxKey string

const userCtxKey ctxKey = "user"

// Authenticator validates admin credentials and issues/verifies JWTs.
//
// For this scaffold the admin identity is sourced from the environment:
//
//	ADMIN_USERNAME       (default: admin)
//	ADMIN_PASSWORD_HASH  (bcrypt hash; takes precedence)
//	ADMIN_PASSWORD       (plaintext fallback for dev only)
//
// In production back this with the management MySQL `admin_users` table.
type Authenticator struct {
	username     string
	passwordHash []byte
	jwtSecret    []byte
	ttl          time.Duration
}

// NewAuthenticator builds the authenticator. If only a plaintext password is
// provided it is hashed in-memory at startup.
func NewAuthenticator(username, passwordHash, plaintext, jwtSecret string, ttl time.Duration) (*Authenticator, error) {
	hash := []byte(passwordHash)
	if len(hash) == 0 {
		if plaintext == "" {
			return nil, errors.New("no admin password configured")
		}
		h, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		hash = h
	}
	if jwtSecret == "" {
		return nil, errors.New("JWT_SECRET is required")
	}
	return &Authenticator{
		username:     username,
		passwordHash: hash,
		jwtSecret:    []byte(jwtSecret),
		ttl:          ttl,
	}, nil
}

// Username returns the configured admin username.
func (a *Authenticator) Username() string { return a.username }

// VerifyPassword checks the admin username + password (constant-time via bcrypt).
func (a *Authenticator) VerifyPassword(username, password string) bool {
	if username != a.username {
		return false
	}
	return bcrypt.CompareHashAndPassword(a.passwordHash, []byte(password)) == nil
}

// Issue returns a signed JWT for the given subject.
func (a *Authenticator) Issue(username string) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   username,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.ttl)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Issuer:    "wordpress-manager",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(a.jwtSecret)
}

// errors used to drive the 2FA login flow.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
)

func (a *Authenticator) parse(token string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return a.jwtSecret, nil
	})
	if err != nil {
		return "", err
	}
	return claims.Subject, nil
}

// Middleware rejects requests without a valid Bearer token.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		user, err := a.parse(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
