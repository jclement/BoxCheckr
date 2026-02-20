package handlers

import (
	"net/http"
)

func (h *Handlers) AdminMachines(w http.ResponseWriter, r *http.Request) {
	filterOwner := r.URL.Query().Get("owner")
	filterMachine := r.URL.Query().Get("machine")

	machines, err := h.db.GetAllMachinesWithOwners(filterOwner, filterMachine)
	if err != nil {
		http.Error(w, "Failed to load machines", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "machines.html", &PageData{
		Title:         "All Machines",
		Active:        "admin",
		Machines:      machines,
		FilterOwner:   filterOwner,
		FilterMachine: filterMachine,
	})
}

func (h *Handlers) AdminDeleteMachine(w http.ResponseWriter, r *http.Request) {
	machineID := r.PathValue("id")
	if machineID == "" {
		http.Error(w, "Machine ID required", http.StatusBadRequest)
		return
	}

	// Verify machine exists
	machine, err := h.db.GetMachine(machineID)
	if err != nil || machine == nil {
		http.Error(w, "Machine not found", http.StatusNotFound)
		return
	}

	// Delete the machine and all its snapshots
	if err := h.db.DeleteMachine(machineID); err != nil {
		http.Error(w, "Failed to delete machine", http.StatusInternalServerError)
		return
	}

	// Redirect back to admin machines list
	http.Redirect(w, r, "/admin/machines", http.StatusSeeOther)
}
