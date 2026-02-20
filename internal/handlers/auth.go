package handlers

import (
	"net/http"

	"github.com/jclement/boxcheckr/internal/middleware"
)

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	// Check if already logged in
	if _, _, ok := h.sessions.GetUser(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	state := middleware.GenerateState()
	session, _ := h.sessions.Get(r)
	session.Values["oauth_state"] = state
	h.sessions.Save(r, w, session)

	url := h.oidc.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (h *Handlers) Callback(w http.ResponseWriter, r *http.Request) {
	session, _ := h.sessions.Get(r)
	expectedState, ok := session.Values["oauth_state"].(string)
	if !ok {
		http.Error(w, "Invalid session state", http.StatusBadRequest)
		return
	}
	delete(session.Values, "oauth_state")

	if r.URL.Query().Get("state") != expectedState {
		http.Error(w, "State mismatch", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "No code in callback", http.StatusBadRequest)
		return
	}

	claims, err := h.oidc.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "Failed to exchange code: "+err.Error(), http.StatusInternalServerError)
		return
	}

	isAdmin := h.oidc.IsAdmin(claims)

	// Upsert user in database
	_, err = h.db.UpsertUser(claims.Subject, claims.Email, claims.Name, isAdmin)
	if err != nil {
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Set session
	if err := h.sessions.SetUser(r, w, claims.Subject, isAdmin); err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	h.sessions.Clear(r, w)
	h.render(w, r, "logout.html", &PageData{Title: "Signed Out"})
}

func (h *Handlers) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "login.html", &PageData{Title: "Sign In"})
}
