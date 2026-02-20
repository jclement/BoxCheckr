package handlers

import (
	"net/http"

	"github.com/jclement/boxcheckr/internal/middleware"
)

func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	// Single query to get all machines with their latest snapshots
	machines, err := h.db.GetMachinesWithLatestByUser(user.ID)
	if err != nil {
		http.Error(w, "Failed to load machines", http.StatusInternalServerError)
		return
	}

	stats, _ := h.db.GetUserDashboardStats(user.ID)

	h.render(w, r, "dashboard.html", &PageData{
		Title:    "Dashboard",
		Active:   "dashboard",
		Stats:    stats,
		Machines: machines,
	})
}
