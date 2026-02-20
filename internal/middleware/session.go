package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/sessions"
)

const (
	SessionName    = "boxcheckr"
	SessionUserID  = "user_id"
	SessionIsAdmin = "is_admin"
)

type SessionStore struct {
	store *sessions.CookieStore
}

func NewSessionStore() *SessionStore {
	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		// Generate a random secret for development
		b := make([]byte, 32)
		rand.Read(b)
		secret = base64.StdEncoding.EncodeToString(b)
	}

	store := sessions.NewCookieStore([]byte(secret))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   strings.HasPrefix(os.Getenv("BASE_URL"), "https://"),
		SameSite: http.SameSiteLaxMode,
	}

	return &SessionStore{store: store}
}

func (s *SessionStore) Get(r *http.Request) (*sessions.Session, error) {
	return s.store.Get(r, SessionName)
}

func (s *SessionStore) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	return session.Save(r, w)
}

func (s *SessionStore) SetUser(r *http.Request, w http.ResponseWriter, userID string, isAdmin bool) error {
	session, err := s.Get(r)
	if err != nil {
		return err
	}
	session.Values[SessionUserID] = userID
	session.Values[SessionIsAdmin] = isAdmin
	return s.Save(r, w, session)
}

func (s *SessionStore) GetUser(r *http.Request) (userID string, isAdmin bool, ok bool) {
	session, err := s.Get(r)
	if err != nil {
		return "", false, false
	}

	userIDVal, ok := session.Values[SessionUserID]
	if !ok {
		return "", false, false
	}

	userID, ok = userIDVal.(string)
	if !ok || userID == "" {
		return "", false, false
	}

	isAdmin, _ = session.Values[SessionIsAdmin].(bool)
	return userID, isAdmin, true
}

func (s *SessionStore) Clear(r *http.Request, w http.ResponseWriter) error {
	session, err := s.Get(r)
	if err != nil {
		return err
	}
	session.Options.MaxAge = -1
	return s.Save(r, w, session)
}

func GenerateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
