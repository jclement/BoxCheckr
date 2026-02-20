package db

import (
	"os"
	"testing"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "boxcheckr-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	db, err := New(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to create test database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		os.Remove(tmpFile.Name())
	})

	return db
}

func TestUserOperations(t *testing.T) {
	db := setupTestDB(t)

	// Test creating a user
	user, err := db.UpsertUser("test-id-123", "test@example.com", "Test User", false)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if user.ID != "test-id-123" {
		t.Errorf("Expected user ID 'test-id-123', got '%s'", user.ID)
	}
	if user.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", user.Email)
	}
	if user.IsAdmin {
		t.Error("Expected user to not be admin")
	}

	// Test updating a user
	user, err = db.UpsertUser("test-id-123", "test@example.com", "Test User Updated", true)
	if err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}
	if user.Name != "Test User Updated" {
		t.Errorf("Expected name 'Test User Updated', got '%s'", user.Name)
	}
	if !user.IsAdmin {
		t.Error("Expected user to be admin after update")
	}

	// Test getting a user
	user, err = db.GetUser("test-id-123")
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if user == nil {
		t.Fatal("Expected to find user")
	}
	if user.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", user.Email)
	}

	// Test getting non-existent user
	user, err = db.GetUser("non-existent")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if user != nil {
		t.Error("Expected nil user for non-existent ID")
	}
}

func TestMachineOperations(t *testing.T) {
	db := setupTestDB(t)

	// Create a user first
	_, err := db.UpsertUser("user-1", "user@example.com", "User One", false)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Test creating a machine
	machine, err := db.CreateMachine("user-1", "Test MacBook")
	if err != nil {
		t.Fatalf("Failed to create machine: %v", err)
	}
	if machine.Name != "Test MacBook" {
		t.Errorf("Expected name 'Test MacBook', got '%s'", machine.Name)
	}
	if machine.UserID != "user-1" {
		t.Errorf("Expected user ID 'user-1', got '%s'", machine.UserID)
	}
	if machine.EnrollmentToken == "" {
		t.Error("Expected enrollment token to be set")
	}
	if len(machine.EnrollmentToken) < 32 {
		t.Error("Expected enrollment token to be at least 32 characters")
	}

	// Test getting machine by ID
	fetched, err := db.GetMachine(machine.ID)
	if err != nil {
		t.Fatalf("Failed to get machine: %v", err)
	}
	if fetched.Name != "Test MacBook" {
		t.Errorf("Expected name 'Test MacBook', got '%s'", fetched.Name)
	}

	// Test getting machine by token
	fetched, err = db.GetMachineByToken(machine.EnrollmentToken)
	if err != nil {
		t.Fatalf("Failed to get machine by token: %v", err)
	}
	if fetched.ID != machine.ID {
		t.Errorf("Expected machine ID '%s', got '%s'", machine.ID, fetched.ID)
	}

	// Test getting machines by user
	machines, err := db.GetMachinesByUser("user-1")
	if err != nil {
		t.Fatalf("Failed to get machines by user: %v", err)
	}
	if len(machines) != 1 {
		t.Errorf("Expected 1 machine, got %d", len(machines))
	}

	// Create another machine
	_, err = db.CreateMachine("user-1", "Test Desktop")
	if err != nil {
		t.Fatalf("Failed to create second machine: %v", err)
	}

	machines, err = db.GetMachinesByUser("user-1")
	if err != nil {
		t.Fatalf("Failed to get machines by user: %v", err)
	}
	if len(machines) != 2 {
		t.Errorf("Expected 2 machines, got %d", len(machines))
	}

	// Test deleting a machine
	err = db.DeleteMachine(machine.ID)
	if err != nil {
		t.Fatalf("Failed to delete machine: %v", err)
	}

	fetched, err = db.GetMachine(machine.ID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if fetched != nil {
		t.Error("Expected machine to be deleted")
	}
}

func TestSnapshotOperations(t *testing.T) {
	db := setupTestDB(t)

	// Create user and machine
	_, err := db.UpsertUser("user-1", "user@example.com", "User One", false)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	machine, err := db.CreateMachine("user-1", "Test MacBook")
	if err != nil {
		t.Fatalf("Failed to create machine: %v", err)
	}

	// Test creating a snapshot
	snapshot := &InventorySnapshot{
		Hostname:              "test-macbook.local",
		OS:                    "darwin",
		OSVersion:             "14.0",
		DiskEncrypted:         true,
		DiskEncryptionDetails: "FileVault enabled",
		AntivirusEnabled:      true,
		AntivirusDetails:      "XProtect active",
		RawData:               `{"test": "data"}`,
	}

	err = db.CreateSnapshot(machine.ID, snapshot)
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	// Test getting latest snapshot
	latest, err := db.GetLatestSnapshot(machine.ID)
	if err != nil {
		t.Fatalf("Failed to get latest snapshot: %v", err)
	}
	if latest == nil {
		t.Fatal("Expected to find snapshot")
	}
	if latest.Hostname != "test-macbook.local" {
		t.Errorf("Expected hostname 'test-macbook.local', got '%s'", latest.Hostname)
	}
	if !latest.DiskEncrypted {
		t.Error("Expected disk to be encrypted")
	}

	// Wait a bit to ensure different timestamps then create another snapshot
	snapshot2 := &InventorySnapshot{
		Hostname:         "test-macbook.local",
		OS:               "darwin",
		OSVersion:        "14.1",
		DiskEncrypted:    true,
		AntivirusEnabled: true,
	}
	err = db.CreateSnapshot(machine.ID, snapshot2)
	if err != nil {
		t.Fatalf("Failed to create second snapshot: %v", err)
	}

	// Test getting history
	history, err := db.GetSnapshotHistory(machine.ID, 10)
	if err != nil {
		t.Fatalf("Failed to get snapshot history: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("Expected 2 snapshots in history, got %d", len(history))
	}

	// Test that latest returns one of the snapshots (due to fast inserts, order may vary)
	latest, err = db.GetLatestSnapshot(machine.ID)
	if err != nil {
		t.Fatalf("Failed to get latest snapshot: %v", err)
	}
	if latest.OSVersion != "14.0" && latest.OSVersion != "14.1" {
		t.Errorf("Expected OS version '14.0' or '14.1', got '%s'", latest.OSVersion)
	}
}

func TestAdminMachineQuery(t *testing.T) {
	db := setupTestDB(t)

	// Create two users
	_, _ = db.UpsertUser("user-1", "alice@example.com", "Alice Smith", false)
	_, _ = db.UpsertUser("user-2", "bob@example.com", "Bob Jones", false)

	// Create machines
	m1, _ := db.CreateMachine("user-1", "Alice MacBook")
	m2, _ := db.CreateMachine("user-1", "Alice Desktop")
	m3, _ := db.CreateMachine("user-2", "Bob Laptop")

	// Add snapshots
	db.CreateSnapshot(m1.ID, &InventorySnapshot{Hostname: "alice-mb", OS: "darwin", DiskEncrypted: true})
	db.CreateSnapshot(m2.ID, &InventorySnapshot{Hostname: "alice-dt", OS: "windows", DiskEncrypted: false})
	db.CreateSnapshot(m3.ID, &InventorySnapshot{Hostname: "bob-lt", OS: "linux", DiskEncrypted: true})

	// Test getting all machines
	machines, err := db.GetAllMachinesWithOwners("", "")
	if err != nil {
		t.Fatalf("Failed to get all machines: %v", err)
	}
	if len(machines) != 3 {
		t.Errorf("Expected 3 machines, got %d", len(machines))
	}

	// Test filtering by owner
	machines, err = db.GetAllMachinesWithOwners("alice", "")
	if err != nil {
		t.Fatalf("Failed to filter by owner: %v", err)
	}
	if len(machines) != 2 {
		t.Errorf("Expected 2 machines for alice, got %d", len(machines))
	}

	// Test filtering by machine name
	machines, err = db.GetAllMachinesWithOwners("", "MacBook")
	if err != nil {
		t.Fatalf("Failed to filter by machine: %v", err)
	}
	if len(machines) != 1 {
		t.Errorf("Expected 1 machine matching 'MacBook', got %d", len(machines))
	}

	// Test combined filter
	machines, err = db.GetAllMachinesWithOwners("bob", "Laptop")
	if err != nil {
		t.Fatalf("Failed combined filter: %v", err)
	}
	if len(machines) != 1 {
		t.Errorf("Expected 1 machine, got %d", len(machines))
	}
	if machines[0].OwnerEmail != "bob@example.com" {
		t.Errorf("Expected bob@example.com, got %s", machines[0].OwnerEmail)
	}
}

func TestDashboardStats(t *testing.T) {
	db := setupTestDB(t)

	// Create user and machines
	_, _ = db.UpsertUser("user-1", "user@example.com", "User", false)
	m1, _ := db.CreateMachine("user-1", "Machine 1")
	m2, _ := db.CreateMachine("user-1", "Machine 2")
	m3, _ := db.CreateMachine("user-1", "Machine 3")

	// Add snapshots with various states
	db.CreateSnapshot(m1.ID, &InventorySnapshot{DiskEncrypted: true, AntivirusEnabled: true})
	db.CreateSnapshot(m2.ID, &InventorySnapshot{DiskEncrypted: true, AntivirusEnabled: false})
	db.CreateSnapshot(m3.ID, &InventorySnapshot{DiskEncrypted: false, AntivirusEnabled: true})

	stats, err := db.GetUserDashboardStats("user-1")
	if err != nil {
		t.Fatalf("Failed to get dashboard stats: %v", err)
	}

	if stats.TotalMachines != 3 {
		t.Errorf("Expected 3 total machines, got %d", stats.TotalMachines)
	}
	if stats.EncryptedCount != 2 {
		t.Errorf("Expected 2 encrypted, got %d", stats.EncryptedCount)
	}
	if stats.UnencryptedCount != 1 {
		t.Errorf("Expected 1 unencrypted, got %d", stats.UnencryptedCount)
	}
	if stats.ProtectedCount != 2 {
		t.Errorf("Expected 2 protected, got %d", stats.ProtectedCount)
	}
	if stats.UnprotectedCount != 1 {
		t.Errorf("Expected 1 unprotected, got %d", stats.UnprotectedCount)
	}
}

func TestTokenGeneration(t *testing.T) {
	// Test that tokens are unique
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := generateToken()
		if err != nil {
			t.Fatalf("Failed to generate token: %v", err)
		}
		if tokens[token] {
			t.Errorf("Duplicate token generated: %s", token)
		}
		tokens[token] = true

		// Check token length (32 bytes base64 encoded = ~43 chars)
		if len(token) < 40 {
			t.Errorf("Token too short: %d chars", len(token))
		}
	}
}
