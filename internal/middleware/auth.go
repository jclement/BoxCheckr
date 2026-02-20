package middleware

import (
	"context"
	"net/http"

	"github.com/jclement/boxcheckr/internal/db"
)

type contextKey string

const (
	ContextKeyUser  contextKey = "user"
	ContextKeyAdmin contextKey = "is_admin"
)

type AuthMiddleware struct {
	sessions *SessionStore
	db       *db.DB
}

func NewAuthMiddleware(sessions *SessionStore, database *db.DB) *AuthMiddleware {
	return &AuthMiddleware{
		sessions: sessions,
		db:       database,
	}
}

func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, isAdmin, ok := m.sessions.GetUser(r)
		if !ok {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}

		user, err := m.db.GetUser(userID)
		if err != nil || user == nil {
			m.sessions.Clear(r, w)
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), ContextKeyUser, user)
		ctx = context.WithValue(ctx, ContextKeyAdmin, isAdmin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *AuthMiddleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, isAdmin, ok := m.sessions.GetUser(r)
		if !ok {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}

		if !isAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		user, err := m.db.GetUser(userID)
		if err != nil || user == nil {
			m.sessions.Clear(r, w)
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), ContextKeyUser, user)
		ctx = context.WithValue(ctx, ContextKeyAdmin, isAdmin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUser(ctx context.Context) *db.User {
	user, _ := ctx.Value(ContextKeyUser).(*db.User)
	return user
}

func IsAdmin(ctx context.Context) bool {
	isAdmin, _ := ctx.Value(ContextKeyAdmin).(bool)
	return isAdmin
}
