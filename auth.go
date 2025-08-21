package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type AuthManager struct {
	db       *Database
	sessions map[string]*Session
	mutex    sync.RWMutex
}

type Session struct {
	Token     string    `json:"token"`
	UserID    int       `json:"user_id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewAuthManager(db *Database) *AuthManager {
	return &AuthManager{
		db:       db,
		sessions: make(map[string]*Session),
	}
}

func (am *AuthManager) generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (am *AuthManager) CreateSession(user *User) (*Session, error) {
	token, err := am.generateToken()
	if err != nil {
		return nil, err
	}

	session := &Session{
		Token:     token,
		UserID:    user.ID,
		Username:  user.Username,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	am.mutex.Lock()
	am.sessions[token] = session
	am.mutex.Unlock()

	return session, nil
}

func (am *AuthManager) ValidateSession(token string) (*Session, error) {
	am.mutex.RLock()
	session, exists := am.sessions[token]
	am.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("invalid session")
	}

	if time.Now().After(session.ExpiresAt) {
		am.mutex.Lock()
		delete(am.sessions, token)
		am.mutex.Unlock()
		return nil, fmt.Errorf("session expired")
	}

	return session, nil
}

func (am *AuthManager) DeleteSession(token string) {
	am.mutex.Lock()
	delete(am.sessions, token)
	am.mutex.Unlock()
}

func (am *AuthManager) ExtractToken(r *http.Request) string {
	// Check Authorization header
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func (am *AuthManager) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := am.ExtractToken(r)
		if token == "" {
			http.Error(w, "Missing authorization token", http.StatusUnauthorized)
			return
		}

		session, err := am.ValidateSession(token)
		if err != nil {
			http.Error(w, "Invalid or expired session", http.StatusUnauthorized)
			return
		}

		// Add session to request context
		r = r.WithContext(contextWithSession(r.Context(), session))
		next(w, r)
	}
}

// Context helpers
type contextKey string

const sessionKey contextKey = "session"

func contextWithSession(ctx context.Context, session *Session) context.Context {
	return context.WithValue(ctx, sessionKey, session)
}

func sessionFromContext(ctx context.Context) *Session {
	if session, ok := ctx.Value(sessionKey).(*Session); ok {
		return session
	}
	return nil
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
