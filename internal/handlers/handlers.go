package handlers

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/jclement/boxcheckr/internal/auth"
	"github.com/jclement/boxcheckr/internal/db"
	"github.com/jclement/boxcheckr/internal/middleware"
)

var funcMap = template.FuncMap{
	"now": time.Now,
}

type Handlers struct {
	db        *db.DB
	oidc      *auth.OIDCProvider
	sessions  *middleware.SessionStore
	baseURL   string
	version   string
	templates map[string]*template.Template
}

func New(database *db.DB, oidc *auth.OIDCProvider, sessions *middleware.SessionStore, baseURL string, version string) *Handlers {
	templates := make(map[string]*template.Template)
	basePath := filepath.Join("web", "templates", "base.html")

	// Parse each page template with the base template
	pageTemplates := []string{
		"dashboard.html",
		"enroll.html",
		"machine.html",
		"login.html",
		"logout.html",
		"error.html",
	}

	for _, page := range pageTemplates {
		pagePath := filepath.Join("web", "templates", page)
		tmpl := template.Must(template.New("").Funcs(funcMap).ParseFiles(basePath, pagePath))
		templates[page] = tmpl
	}

	// Admin templates
	adminTemplates := []string{
		"machines.html",
		"share.html",
	}

	for _, page := range adminTemplates {
		pagePath := filepath.Join("web", "templates", "admin", page)
		tmpl := template.Must(template.New("").Funcs(funcMap).ParseFiles(basePath, pagePath))
		templates[page] = tmpl
	}

	// Public templates (for shared links, no auth header)
	publicBasePath := filepath.Join("web", "templates", "public_base.html")
	publicTemplates := []string{
		"shared.html",
	}

	for _, page := range publicTemplates {
		pagePath := filepath.Join("web", "templates", "public", page)
		tmpl := template.Must(template.New("").Funcs(funcMap).ParseFiles(publicBasePath, pagePath))
		templates[page] = tmpl
	}

	return &Handlers{
		db:        database,
		oidc:      oidc,
		sessions:  sessions,
		baseURL:   baseURL,
		version:   version,
		templates: templates,
	}
}

type PageData struct {
	Title   string
	Active  string
	User    *db.User
	IsAdmin bool
	BaseURL string
	Version string

	// Page-specific data
	Stats         *db.DashboardStats
	Machines      interface{}
	Machine       *db.Machine
	Latest        *db.InventorySnapshot
	History       []db.InventorySnapshot
	Notes         []db.MachineNote
	Success       bool
	FilterOwner   string
	FilterMachine string

	// Share links
	ShareLinks []db.ShareLink
	ShareLink  *db.ShareLink
	NewLinkID  string

	// Error page data
	ErrorCode    int
	ErrorMessage string
}

func (h *Handlers) render(w http.ResponseWriter, r *http.Request, name string, data *PageData) {
	if data == nil {
		data = &PageData{}
	}

	user := middleware.GetUser(r.Context())
	if user != nil {
		data.User = user
		data.IsAdmin = middleware.IsAdmin(r.Context())
	}
	data.BaseURL = h.baseURL
	data.Version = h.version

	// Get the template for this page
	tmpl, ok := h.templates[name]
	if !ok {
		log.Printf("Template not found: %s", name)
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	// Render to buffer first to catch template errors before writing headers
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base.html", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func (h *Handlers) renderError(w http.ResponseWriter, r *http.Request, code int, message string) {
	w.WriteHeader(code)
	h.render(w, r, "error.html", &PageData{
		Title:        http.StatusText(code),
		ErrorCode:    code,
		ErrorMessage: message,
	})
}

func (h *Handlers) renderPublic(w http.ResponseWriter, name string, data *PageData) {
	if data == nil {
		data = &PageData{}
	}
	data.BaseURL = h.baseURL
	data.Version = h.version

	tmpl, ok := h.templates[name]
	if !ok {
		log.Printf("Template not found: %s", name)
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base.html", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func (h *Handlers) NotFound(w http.ResponseWriter, r *http.Request) {
	h.renderError(w, r, http.StatusNotFound, "")
}

func (h *Handlers) Forbidden(w http.ResponseWriter, r *http.Request) {
	h.renderError(w, r, http.StatusForbidden, "")
}
