package handlers

import (
	"net/http"

	"github.com/jclement/boxcheckr/internal/scripts"
)

// MachineScript serves the agent script - NO AUTH REQUIRED
// This endpoint is called by curl/wget from the user's terminal
func (h *Handlers) MachineScript(w http.ResponseWriter, r *http.Request) {
	machineID := r.PathValue("id")
	machine, err := h.db.GetMachine(machineID)
	if err != nil || machine == nil {
		http.Error(w, "Machine not found", http.StatusNotFound)
		return
	}

	// Get machine owner for email in script comments
	owner, _ := h.db.GetUser(machine.UserID)
	email := ""
	if owner != nil {
		email = owner.Email
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "onetime"
	}

	// Use OS from query param if provided, otherwise detect from User-Agent
	osType := r.URL.Query().Get("os")
	if osType == "" {
		osType = scripts.DetectOS(r.Header.Get("User-Agent"))
	}

	// Set appropriate content type
	if osType == "windows" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "inline; filename=boxcheckr-agent.ps1")
	} else {
		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
		w.Header().Set("Content-Disposition", "inline; filename=boxcheckr-agent.sh")
	}

	data := scripts.ScriptData{
		Token:     machine.EnrollmentToken,
		ServerURL: h.baseURL,
		Email:     email,
		Mode:      mode,
		MachineID: machineID,
	}

	if err := scripts.GenerateScript(w, osType, data); err != nil {
		http.Error(w, "Failed to generate script", http.StatusInternalServerError)
	}
}
