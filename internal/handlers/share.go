package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jclement/boxcheckr/internal/middleware"
)

// CreateShareLink creates a new time-limited share link (admin only)
func (h *Handlers) CreateShareLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse duration from form (in hours)
	hoursStr := r.FormValue("hours")
	hours := 24 // default 1 day
	if hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 && h <= 8760 { // max 1 year
			hours = h
		}
	}

	expiresAt := time.Now().Add(time.Duration(hours) * time.Hour)

	link, err := h.db.CreateShareLink(user.ID, expiresAt)
	if err != nil {
		http.Error(w, "Failed to create share link", http.StatusInternalServerError)
		return
	}

	// Redirect to admin page with the new link highlighted
	http.Redirect(w, r, "/admin/share?new="+link.ID, http.StatusSeeOther)
}

// DeleteShareLink deletes a share link (admin only)
func (h *Handlers) DeleteShareLink(w http.ResponseWriter, r *http.Request) {
	linkID := r.PathValue("id")
	if linkID == "" {
		http.Error(w, "Link ID required", http.StatusBadRequest)
		return
	}

	if err := h.db.DeleteShareLink(linkID); err != nil {
		http.Error(w, "Failed to delete share link", http.StatusInternalServerError)
		return
	}

	// HTMX request: return empty response (row will be removed via hx-swap)
	if r.Header.Get("HX-Request") == "true" {
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, "/admin/share", http.StatusSeeOther)
}

// AdminShareLinks shows the admin page for managing share links
func (h *Handlers) AdminShareLinks(w http.ResponseWriter, r *http.Request) {
	links, err := h.db.GetAllShareLinks()
	if err != nil {
		http.Error(w, "Failed to load share links", http.StatusInternalServerError)
		return
	}

	newLinkID := r.URL.Query().Get("new")

	h.render(w, r, "share.html", &PageData{
		Title:      "Share Links",
		Active:     "share",
		ShareLinks: links,
		NewLinkID:  newLinkID,
	})
}

// ViewSharedInventory displays the public shared view (no auth required)
func (h *Handlers) ViewSharedInventory(w http.ResponseWriter, r *http.Request) {
	linkID := r.PathValue("id")
	if linkID == "" {
		h.renderError(w, r, http.StatusNotFound, "Share link not found")
		return
	}

	// Get and validate the share link
	link, err := h.db.GetValidShareLink(linkID)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Failed to validate share link")
		return
	}
	if link == nil {
		h.renderError(w, r, http.StatusNotFound, "Share link not found or has expired")
		return
	}

	// Get all machines with their latest snapshots and notes
	machines, err := h.db.GetAllMachinesWithOwners("", "")
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, "Failed to load inventory")
		return
	}

	h.renderPublic(w, "shared.html", &PageData{
		Title:     "Shared Inventory",
		Machines:  machines,
		ShareLink: link,
	})
}
