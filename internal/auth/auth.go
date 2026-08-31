package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const cookieName = "cineroom_session"

type User struct{ ID, Email, Username string }
type Manager struct {
	secret        []byte
	secureCookies bool
}
type contextKey struct{}

func New(secret []byte, secureCookies bool) *Manager {
	return &Manager{secret: secret, secureCookies: secureCookies}
}
func HashPassword(password string) (string, error) {
	if len(password) < 6 {
		return "", errors.New("password must contain at least 6 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
func (m *Manager) SetSession(w http.ResponseWriter, user User) error {
	// Access token: short-lived JWT (15 minutes)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": user.ID, "email": user.Email, "username": user.Username, "exp": time.Now().Add(15 * time.Minute).Unix(), "iat": time.Now().Unix()})
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: signed, Path: "/", HttpOnly: true, Secure: m.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: 15 * 60})
	return nil
}
func (m *Manager) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: m.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (m *Manager) Current(r *http.Request) (User, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return User{}, err
	}
	token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return User{}, errors.New("invalid session")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return User{}, errors.New("invalid claims")
	}
	id, _ := claims.GetSubject()
	email, _ := claims["email"].(string)
	username, _ := claims["username"].(string)
	if id == "" || email == "" || username == "" {
		return User{}, errors.New("invalid claims")
	}
	return User{ID: id, Email: email, Username: username}, nil
}
func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := m.Current(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, u)))
	})
}
func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(contextKey{}).(User)
	return u, ok
}
func NormalizeEmail(email string) string       { return strings.ToLower(strings.TrimSpace(email)) }
func NormalizeUsername(username string) string { return strings.ToLower(strings.TrimSpace(username)) }
func ValidUsername(username string) bool {
	if len(username) < 3 || len(username) > 32 {
		return false
	}
	for _, char := range username {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_') {
			return false
		}
	}
	return true
}
