package main

import (
	"log"
	"net/http"
	"os"

	"github.com/jclement/boxcheckr/internal/auth"
	"github.com/jclement/boxcheckr/internal/db"
	"github.com/jclement/boxcheckr/internal/handlers"
	"github.com/jclement/boxcheckr/internal/middleware"
)

// Version is set at build time via ldflags
var Version = "dev"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./boxcheckr.db"
	}

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	oidcProvider, err := auth.NewOIDCProvider(baseURL)
	if err != nil {
		log.Fatalf("Failed to initialize OIDC provider: %v", err)
	}

	sessionStore := middleware.NewSessionStore()
	authMiddleware := middleware.NewAuthMiddleware(sessionStore, database)

	h := handlers.New(database, oidcProvider, sessionStore, baseURL, Version)

	mux := http.NewServeMux()

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	// Auth routes
	mux.HandleFunc("GET /auth/login", h.Login)
	mux.HandleFunc("GET /auth/callback", h.Callback)
	mux.HandleFunc("GET /auth/logout", h.Logout)

	// User routes (require auth)
	mux.Handle("GET /", authMiddleware.RequireAuth(http.HandlerFunc(h.Dashboard)))
	mux.Handle("GET /enroll", authMiddleware.RequireAuth(http.HandlerFunc(h.EnrollPage)))
	mux.Handle("POST /enroll", authMiddleware.RequireAuth(http.HandlerFunc(h.EnrollMachine)))
	mux.Handle("GET /machines/{id}", authMiddleware.RequireAuth(http.HandlerFunc(h.MachineDetail)))
	mux.Handle("POST /machines/{id}/delete", authMiddleware.RequireAuth(http.HandlerFunc(h.DeleteMachine)))

	// Script endpoint - NO AUTH (called by curl from terminal)
	mux.HandleFunc("GET /machines/{id}/script", h.MachineScript)

	// Machine notes (admin only)
	mux.Handle("POST /machines/{id}/notes", authMiddleware.RequireAdmin(http.HandlerFunc(h.AddMachineNote)))
	mux.Handle("POST /machines/{id}/notes/{noteId}/delete", authMiddleware.RequireAdmin(http.HandlerFunc(h.DeleteMachineNote)))

	// Admin routes (require admin)
	mux.Handle("GET /admin/machines", authMiddleware.RequireAdmin(http.HandlerFunc(h.AdminMachines)))
	mux.Handle("POST /admin/machines/{id}/delete", authMiddleware.RequireAdmin(http.HandlerFunc(h.AdminDeleteMachine)))
	mux.Handle("GET /admin/share", authMiddleware.RequireAdmin(http.HandlerFunc(h.AdminShareLinks)))
	mux.Handle("POST /admin/share", authMiddleware.RequireAdmin(http.HandlerFunc(h.CreateShareLink)))
	mux.Handle("POST /admin/share/{id}/delete", authMiddleware.RequireAdmin(http.HandlerFunc(h.DeleteShareLink)))

	// Public share link view (NO AUTH)
	mux.HandleFunc("GET /share/{id}", h.ViewSharedInventory)

	// API routes (token auth)
	mux.HandleFunc("POST /api/v1/inventory", h.SubmitInventory)

	log.Printf("BoxCheckr starting on port %s", port)
	log.Printf("Base URL: %s", baseURL)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
