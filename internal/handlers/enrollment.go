package handlers

import (
	"net/http"
	"strconv"

	"github.com/jclement/boxcheckr/internal/middleware"
)

func (h *Handlers) EnrollPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "enroll.html", &PageData{
		Title:  "Enroll Machine",
		Active: "enroll",
	})
}

func (h *Handlers) EnrollMachine(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Machine name is required", http.StatusBadRequest)
		return
	}

	machine, err := h.db.CreateMachine(user.ID, name)
	if err != nil {
		http.Error(w, "Failed to create machine", http.StatusInternalServerError)
		return
	}

	// Redirect to machine detail page to show script download
	http.Redirect(w, r, "/machines/"+machine.ID, http.StatusSeeOther)
}

func (h *Handlers) MachineDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	machineID := r.PathValue("id")
	machine, err := h.db.GetMachine(machineID)
	if err != nil || machine == nil {
		h.renderError(w, r, http.StatusNotFound, "Machine not found")
		return
	}

	// Check ownership (unless admin)
	if machine.UserID != user.ID && !middleware.IsAdmin(r.Context()) {
		h.renderError(w, r, http.StatusForbidden, "You don't have permission to view this machine")
		return
	}

	latest, _ := h.db.GetLatestSnapshot(machineID)
	history, _ := h.db.GetSnapshotHistory(machineID, 20)
	notes, _ := h.db.GetMachineNotes(machineID)

	h.render(w, r, "machine.html", &PageData{
		Title:   machine.Name,
		Active:  "dashboard",
		Machine: machine,
		Latest:  latest,
		History: history,
		Notes:   notes,
	})
}

func (h *Handlers) DeleteMachine(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	machineID := r.PathValue("id")
	machine, err := h.db.GetMachine(machineID)
	if err != nil || machine == nil {
		h.renderError(w, r, http.StatusNotFound, "Machine not found")
		return
	}

	// Check ownership (unless admin)
	if machine.UserID != user.ID && !middleware.IsAdmin(r.Context()) {
		h.renderError(w, r, http.StatusForbidden, "You don't have permission to delete this machine")
		return
	}

	if err := h.db.DeleteMachine(machineID); err != nil {
		http.Error(w, "Failed to delete machine", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// AddMachineNote adds a note to a machine (admin only)
func (h *Handlers) AddMachineNote(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	machineID := r.PathValue("id")
	machine, err := h.db.GetMachine(machineID)
	if err != nil || machine == nil {
		h.renderError(w, r, http.StatusNotFound, "Machine not found")
		return
	}

	content := r.FormValue("content")
	if content == "" {
		http.Error(w, "Note content is required", http.StatusBadRequest)
		return
	}

	if _, err := h.db.CreateMachineNote(machineID, user.ID, content); err != nil {
		http.Error(w, "Failed to add note", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/machines/"+machineID, http.StatusSeeOther)
}

// DeleteMachineNote deletes a note from a machine (admin only)
func (h *Handlers) DeleteMachineNote(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	machineID := r.PathValue("id")
	noteIDStr := r.PathValue("noteId")
	noteID, err := strconv.ParseInt(noteIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid note ID", http.StatusBadRequest)
		return
	}

	// Verify the note exists and belongs to this machine
	note, err := h.db.GetMachineNote(noteID)
	if err != nil || note == nil || note.MachineID != machineID {
		h.renderError(w, r, http.StatusNotFound, "Note not found")
		return
	}

	if err := h.db.DeleteMachineNote(noteID); err != nil {
		http.Error(w, "Failed to delete note", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/machines/"+machineID, http.StatusSeeOther)
}
