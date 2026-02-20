package handlers

import (
	"net/http"

	"github.com/jclement/boxcheckr/internal/middleware"
)

type MachineWithLatest struct {
	ID              string
	UserID          string
	Name            string
	EnrollmentToken string
	CreatedAt       interface{}
	Latest          *struct {
		OS               string
		OSVersion        string
		Hostname         string
		DiskEncrypted    bool
		AntivirusEnabled bool
		CollectedAt      interface{}
	}
}

func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	machines, err := h.db.GetMachinesByUser(user.ID)
	if err != nil {
		http.Error(w, "Failed to load machines", http.StatusInternalServerError)
		return
	}

	// Build machines with latest snapshots
	type machineData struct {
		ID              string
		UserID          string
		Name            string
		EnrollmentToken string
		CreatedAt       interface{}
		Latest          interface{}
	}

	var machineList []machineData
	for _, m := range machines {
		latest, _ := h.db.GetLatestSnapshot(m.ID)
		machineList = append(machineList, machineData{
			ID:              m.ID,
			UserID:          m.UserID,
			Name:            m.Name,
			EnrollmentToken: m.EnrollmentToken,
			CreatedAt:       m.CreatedAt,
			Latest:          latest,
		})
	}

	stats, _ := h.db.GetUserDashboardStats(user.ID)

	h.render(w, r, "dashboard.html", &PageData{
		Title:    "Dashboard",
		Active:   "dashboard",
		Stats:    stats,
		Machines: machineList,
	})
}
