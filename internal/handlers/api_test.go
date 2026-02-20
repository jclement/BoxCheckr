package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jclement/boxcheckr/internal/db"
	"github.com/jclement/boxcheckr/internal/middleware"
)

func setupTestHandlers(t *testing.T) (*Handlers, *db.DB, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "boxcheckr-handlers-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	database, err := db.New(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to create database: %v", err)
	}

	sessions := middleware.NewSessionStore()

	// Create handlers without OIDC (we'll test API endpoints that don't need it)
	h := &Handlers{
		db:       database,
		sessions: sessions,
		baseURL:  "http://localhost:8080",
	}

	cleanup := func() {
		database.Close()
		os.Remove(tmpFile.Name())
	}

	return h, database, cleanup
}

func TestSubmitInventory(t *testing.T) {
	h, database, cleanup := setupTestHandlers(t)
	defer cleanup()

	// Create a user and machine
	_, err := database.UpsertUser("test-user", "test@example.com", "Test User", false)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	machine, err := database.CreateMachine("test-user", "Test Machine")
	if err != nil {
		t.Fatalf("Failed to create machine: %v", err)
	}

	tests := []struct {
		name           string
		authHeader     string
		body           interface{}
		expectedStatus int
	}{
		{
			name:           "missing auth header",
			authHeader:     "",
			body:           map[string]interface{}{},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid auth format",
			authHeader:     "InvalidFormat",
			body:           map[string]interface{}{},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid token",
			authHeader:     "Bearer invalid-token",
			body:           map[string]interface{}{},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:       "valid submission",
			authHeader: "Bearer " + machine.EnrollmentToken,
			body: map[string]interface{}{
				"hostname":               "test-host",
				"os":                     "darwin",
				"os_version":             "14.0",
				"disk_encrypted":         true,
				"disk_encryption_details": "FileVault enabled",
				"antivirus_enabled":      true,
				"antivirus_details":      "XProtect active",
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			h.SubmitInventory(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}

	// Verify the snapshot was created
	snapshot, err := database.GetLatestSnapshot(machine.ID)
	if err != nil {
		t.Fatalf("Failed to get snapshot: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected snapshot to exist")
	}
	if snapshot.Hostname != "test-host" {
		t.Errorf("Expected hostname 'test-host', got '%s'", snapshot.Hostname)
	}
	if snapshot.OS != "darwin" {
		t.Errorf("Expected OS 'darwin', got '%s'", snapshot.OS)
	}
	if !snapshot.DiskEncrypted {
		t.Error("Expected disk to be encrypted")
	}
}

func TestSubmitInventoryInvalidJSON(t *testing.T) {
	h, database, cleanup := setupTestHandlers(t)
	defer cleanup()

	_, _ = database.UpsertUser("test-user", "test@example.com", "Test User", false)
	machine, _ := database.CreateMachine("test-user", "Test Machine")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+machine.EnrollmentToken)

	rr := httptest.NewRecorder()
	h.SubmitInventory(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestSubmitInventoryMultipleSnapshots(t *testing.T) {
	h, database, cleanup := setupTestHandlers(t)
	defer cleanup()

	_, _ = database.UpsertUser("test-user", "test@example.com", "Test User", false)
	machine, _ := database.CreateMachine("test-user", "Test Machine")

	// Submit multiple snapshots
	for i := 0; i < 3; i++ {
		payload := map[string]interface{}{
			"hostname":         "test-host",
			"os":               "darwin",
			"os_version":       "14.0",
			"disk_encrypted":   true,
			"antivirus_enabled": true,
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+machine.EnrollmentToken)

		rr := httptest.NewRecorder()
		h.SubmitInventory(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Submission %d: Expected status %d, got %d", i+1, http.StatusOK, rr.Code)
		}
	}

	// Verify all snapshots were created
	history, err := database.GetSnapshotHistory(machine.ID, 10)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	if len(history) != 3 {
		t.Errorf("Expected 3 snapshots, got %d", len(history))
	}
}
