package db

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

type Machine struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	Name            string    `json:"name"`
	EnrollmentToken string    `json:"enrollment_token"`
	CreatedAt       time.Time `json:"created_at"`
}

type InventorySnapshot struct {
	ID                    int64     `json:"id"`
	MachineID             string    `json:"machine_id"`
	CollectedAt           time.Time `json:"collected_at"`
	Hostname              string    `json:"hostname"`
	OS                    string    `json:"os"`
	OSVersion             string    `json:"os_version"`
	DiskEncrypted         bool      `json:"disk_encrypted"`
	DiskEncryptionDetails string    `json:"disk_encryption_details"`
	AntivirusEnabled      bool      `json:"antivirus_enabled"`
	AntivirusDetails      string    `json:"antivirus_details"`
	FirewallEnabled       bool      `json:"firewall_enabled"`
	FirewallDetails       string    `json:"firewall_details"`
	ScreenLockEnabled     bool      `json:"screen_lock_enabled"`
	ScreenLockTimeout     int       `json:"screen_lock_timeout"`
	ScreenLockDetails     string    `json:"screen_lock_details"`
	RawData               string    `json:"raw_data"`
}

// MachineWithOwner combines machine info with owner details for admin views
type MachineWithOwner struct {
	Machine
	OwnerEmail string              `json:"owner_email"`
	OwnerName  string              `json:"owner_name"`
	Latest     *InventorySnapshot  `json:"latest,omitempty"`
}
