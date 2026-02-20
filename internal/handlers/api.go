package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/jclement/boxcheckr/internal/db"
)

type InventoryPayload struct {
	Hostname              string `json:"hostname"`
	OS                    string `json:"os"`
	OSVersion             string `json:"os_version"`
	DiskEncrypted         bool   `json:"disk_encrypted"`
	DiskEncryptionDetails string `json:"disk_encryption_details"`
	AntivirusEnabled      bool   `json:"antivirus_enabled"`
	AntivirusDetails      string `json:"antivirus_details"`
	FirewallEnabled       bool   `json:"firewall_enabled"`
	FirewallDetails       string `json:"firewall_details"`
	ScreenLockEnabled     bool   `json:"screen_lock_enabled"`
	ScreenLockTimeout     int    `json:"screen_lock_timeout"`
	ScreenLockDetails     string `json:"screen_lock_details"`
}

func (h *Handlers) SubmitInventory(w http.ResponseWriter, r *http.Request) {
	// Extract token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == authHeader {
		http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
		return
	}

	// Look up machine by token
	machine, err := h.db.GetMachineByToken(token)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if machine == nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Parse payload
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var payload InventoryPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create snapshot
	snapshot := &db.InventorySnapshot{
		Hostname:              payload.Hostname,
		OS:                    payload.OS,
		OSVersion:             payload.OSVersion,
		DiskEncrypted:         payload.DiskEncrypted,
		DiskEncryptionDetails: payload.DiskEncryptionDetails,
		AntivirusEnabled:      payload.AntivirusEnabled,
		AntivirusDetails:      payload.AntivirusDetails,
		FirewallEnabled:       payload.FirewallEnabled,
		FirewallDetails:       payload.FirewallDetails,
		ScreenLockEnabled:     payload.ScreenLockEnabled,
		ScreenLockTimeout:     payload.ScreenLockTimeout,
		ScreenLockDetails:     payload.ScreenLockDetails,
		RawData:               string(body),
	}

	if err := h.db.CreateSnapshot(machine.ID, snapshot); err != nil {
		http.Error(w, "Failed to save snapshot", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"machine": machine.Name,
	})
}
