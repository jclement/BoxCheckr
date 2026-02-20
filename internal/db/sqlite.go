package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		is_admin BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS machines (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id),
		name TEXT NOT NULL,
		enrollment_token TEXT UNIQUE NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS inventory_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		machine_id TEXT NOT NULL REFERENCES machines(id),
		collected_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		hostname TEXT,
		os TEXT,
		os_version TEXT,
		disk_encrypted BOOLEAN,
		disk_encryption_details TEXT,
		antivirus_enabled BOOLEAN,
		antivirus_details TEXT,
		firewall_enabled BOOLEAN,
		firewall_details TEXT,
		screen_lock_enabled BOOLEAN,
		screen_lock_timeout INTEGER,
		screen_lock_details TEXT,
		raw_data TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_machines_user_id ON machines(user_id);
	CREATE INDEX IF NOT EXISTS idx_machines_enrollment_token ON machines(enrollment_token);
	CREATE INDEX IF NOT EXISTS idx_inventory_snapshots_machine_id ON inventory_snapshots(machine_id);
	CREATE INDEX IF NOT EXISTS idx_inventory_snapshots_collected_at ON inventory_snapshots(collected_at);

	CREATE TABLE IF NOT EXISTS machine_notes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
		author_id TEXT NOT NULL REFERENCES users(id),
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_machine_notes_machine_id ON machine_notes(machine_id);
	`

	_, err := db.conn.Exec(schema)
	return err
}

// User operations

func (db *DB) UpsertUser(id, email, name string, isAdmin bool) (*User, error) {
	_, err := db.conn.Exec(`
		INSERT INTO users (id, email, name, is_admin) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET email = excluded.email, name = excluded.name, is_admin = excluded.is_admin
	`, id, email, name, isAdmin)
	if err != nil {
		return nil, err
	}
	return db.GetUser(id)
}

func (db *DB) GetUser(id string) (*User, error) {
	var u User
	err := db.conn.QueryRow(`SELECT id, email, name, is_admin, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.IsAdmin, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Machine operations

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (db *DB) CreateMachine(userID, name string) (*Machine, error) {
	id := uuid.New().String()
	token, err := generateToken()
	if err != nil {
		return nil, err
	}

	_, err = db.conn.Exec(`
		INSERT INTO machines (id, user_id, name, enrollment_token) VALUES (?, ?, ?, ?)
	`, id, userID, name, token)
	if err != nil {
		return nil, err
	}

	return db.GetMachine(id)
}

func (db *DB) GetMachine(id string) (*Machine, error) {
	var m Machine
	err := db.conn.QueryRow(`SELECT id, user_id, name, enrollment_token, created_at FROM machines WHERE id = ?`, id).
		Scan(&m.ID, &m.UserID, &m.Name, &m.EnrollmentToken, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (db *DB) GetMachineByToken(token string) (*Machine, error) {
	var m Machine
	err := db.conn.QueryRow(`SELECT id, user_id, name, enrollment_token, created_at FROM machines WHERE enrollment_token = ?`, token).
		Scan(&m.ID, &m.UserID, &m.Name, &m.EnrollmentToken, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (db *DB) GetMachinesByUser(userID string) ([]Machine, error) {
	rows, err := db.conn.Query(`
		SELECT m.id, m.user_id, m.name, m.enrollment_token, m.created_at
		FROM machines m
		LEFT JOIN (
			SELECT machine_id, MAX(collected_at) as last_update
			FROM inventory_snapshots
			GROUP BY machine_id
		) s ON m.id = s.machine_id
		WHERE m.user_id = ?
		ORDER BY COALESCE(s.last_update, m.created_at) DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var machines []Machine
	for rows.Next() {
		var m Machine
		if err := rows.Scan(&m.ID, &m.UserID, &m.Name, &m.EnrollmentToken, &m.CreatedAt); err != nil {
			return nil, err
		}
		machines = append(machines, m)
	}
	return machines, rows.Err()
}

// GetMachinesWithLatestByUser returns all machines for a user with their latest snapshot in a single query
func (db *DB) GetMachinesWithLatestByUser(userID string) ([]MachineWithLatest, error) {
	rows, err := db.conn.Query(`
		SELECT
			m.id, m.user_id, m.name, m.enrollment_token, m.created_at,
			s.id, s.collected_at, s.hostname, s.os, s.os_version,
			s.disk_encrypted, s.disk_encryption_details, s.antivirus_enabled, s.antivirus_details,
			s.firewall_enabled, s.firewall_details, s.screen_lock_enabled, s.screen_lock_timeout, s.screen_lock_details
		FROM machines m
		LEFT JOIN inventory_snapshots s ON s.id = (
			SELECT id FROM inventory_snapshots
			WHERE machine_id = m.id
			ORDER BY collected_at DESC
			LIMIT 1
		)
		WHERE m.user_id = ?
		ORDER BY COALESCE(s.collected_at, m.created_at) DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var machines []MachineWithLatest
	for rows.Next() {
		var mwl MachineWithLatest
		var snapshotID sql.NullInt64
		var collectedAt sql.NullTime
		var hostname, os, osVersion sql.NullString
		var diskEncrypted, avEnabled sql.NullBool
		var diskDetails, avDetails sql.NullString
		var fwEnabled, slEnabled sql.NullBool
		var fwDetails, slDetails sql.NullString
		var slTimeout sql.NullInt64

		if err := rows.Scan(
			&mwl.ID, &mwl.UserID, &mwl.Name, &mwl.EnrollmentToken, &mwl.CreatedAt,
			&snapshotID, &collectedAt, &hostname, &os, &osVersion,
			&diskEncrypted, &diskDetails, &avEnabled, &avDetails,
			&fwEnabled, &fwDetails, &slEnabled, &slTimeout, &slDetails,
		); err != nil {
			return nil, err
		}

		if snapshotID.Valid {
			mwl.Latest = &InventorySnapshot{
				ID:                    snapshotID.Int64,
				MachineID:             mwl.ID,
				CollectedAt:           collectedAt.Time,
				Hostname:              hostname.String,
				OS:                    os.String,
				OSVersion:             osVersion.String,
				DiskEncrypted:         diskEncrypted.Bool,
				DiskEncryptionDetails: diskDetails.String,
				AntivirusEnabled:      avEnabled.Bool,
				AntivirusDetails:      avDetails.String,
				FirewallEnabled:       fwEnabled.Bool,
				FirewallDetails:       fwDetails.String,
				ScreenLockEnabled:     slEnabled.Bool,
				ScreenLockTimeout:     int(slTimeout.Int64),
				ScreenLockDetails:     slDetails.String,
			}
		}

		machines = append(machines, mwl)
	}
	return machines, rows.Err()
}

func (db *DB) DeleteMachine(id string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete snapshots first
	if _, err := tx.Exec(`DELETE FROM inventory_snapshots WHERE machine_id = ?`, id); err != nil {
		return err
	}

	// Delete machine
	if _, err := tx.Exec(`DELETE FROM machines WHERE id = ?`, id); err != nil {
		return err
	}

	return tx.Commit()
}

// Admin: Get all machines with owner info and latest snapshot in a single query
func (db *DB) GetAllMachinesWithOwners(filterOwner, filterMachine string) ([]MachineWithOwner, error) {
	query := `
		SELECT
			m.id, m.user_id, m.name, m.enrollment_token, m.created_at,
			u.email, u.name,
			s.id, s.collected_at, s.hostname, s.os, s.os_version,
			s.disk_encrypted, s.disk_encryption_details, s.antivirus_enabled, s.antivirus_details,
			s.firewall_enabled, s.firewall_details, s.screen_lock_enabled, s.screen_lock_timeout, s.screen_lock_details
		FROM machines m
		JOIN users u ON m.user_id = u.id
		LEFT JOIN inventory_snapshots s ON s.id = (
			SELECT id FROM inventory_snapshots
			WHERE machine_id = m.id
			ORDER BY collected_at DESC
			LIMIT 1
		)
		WHERE 1=1
	`
	args := []interface{}{}

	if filterOwner != "" {
		query += ` AND (u.email LIKE ? OR u.name LIKE ?)`
		args = append(args, "%"+filterOwner+"%", "%"+filterOwner+"%")
	}
	if filterMachine != "" {
		query += ` AND m.name LIKE ?`
		args = append(args, "%"+filterMachine+"%")
	}

	query += ` ORDER BY LOWER(u.name), LOWER(u.email), COALESCE(s.collected_at, m.created_at) DESC`

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var machines []MachineWithOwner
	for rows.Next() {
		var m MachineWithOwner
		var snapshotID sql.NullInt64
		var collectedAt sql.NullTime
		var hostname, os, osVersion sql.NullString
		var diskEncrypted, avEnabled sql.NullBool
		var diskDetails, avDetails sql.NullString
		var fwEnabled, slEnabled sql.NullBool
		var fwDetails, slDetails sql.NullString
		var slTimeout sql.NullInt64

		if err := rows.Scan(
			&m.ID, &m.UserID, &m.Name, &m.EnrollmentToken, &m.CreatedAt,
			&m.OwnerEmail, &m.OwnerName,
			&snapshotID, &collectedAt, &hostname, &os, &osVersion,
			&diskEncrypted, &diskDetails, &avEnabled, &avDetails,
			&fwEnabled, &fwDetails, &slEnabled, &slTimeout, &slDetails,
		); err != nil {
			return nil, err
		}

		if snapshotID.Valid {
			m.Latest = &InventorySnapshot{
				ID:                    snapshotID.Int64,
				MachineID:             m.ID,
				CollectedAt:           collectedAt.Time,
				Hostname:              hostname.String,
				OS:                    os.String,
				OSVersion:             osVersion.String,
				DiskEncrypted:         diskEncrypted.Bool,
				DiskEncryptionDetails: diskDetails.String,
				AntivirusEnabled:      avEnabled.Bool,
				AntivirusDetails:      avDetails.String,
				FirewallEnabled:       fwEnabled.Bool,
				FirewallDetails:       fwDetails.String,
				ScreenLockEnabled:     slEnabled.Bool,
				ScreenLockTimeout:     int(slTimeout.Int64),
				ScreenLockDetails:     slDetails.String,
			}
		}

		machines = append(machines, m)
	}

	// Notes still fetched separately (one-to-many relationship)
	for i := range machines {
		notes, _ := db.GetMachineNotes(machines[i].ID)
		machines[i].Notes = notes
	}

	return machines, rows.Err()
}

// Inventory operations

func (db *DB) CreateSnapshot(machineID string, snapshot *InventorySnapshot) error {
	_, err := db.conn.Exec(`
		INSERT INTO inventory_snapshots
		(machine_id, hostname, os, os_version, disk_encrypted, disk_encryption_details, antivirus_enabled, antivirus_details, firewall_enabled, firewall_details, screen_lock_enabled, screen_lock_timeout, screen_lock_details, raw_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, machineID, snapshot.Hostname, snapshot.OS, snapshot.OSVersion,
		snapshot.DiskEncrypted, snapshot.DiskEncryptionDetails,
		snapshot.AntivirusEnabled, snapshot.AntivirusDetails,
		snapshot.FirewallEnabled, snapshot.FirewallDetails,
		snapshot.ScreenLockEnabled, snapshot.ScreenLockTimeout, snapshot.ScreenLockDetails,
		snapshot.RawData)
	return err
}

func (db *DB) GetLatestSnapshot(machineID string) (*InventorySnapshot, error) {
	var s InventorySnapshot
	var firewallEnabled, screenLockEnabled sql.NullBool
	var screenLockTimeout sql.NullInt64
	var firewallDetails, screenLockDetails sql.NullString
	err := db.conn.QueryRow(`
		SELECT id, machine_id, collected_at, hostname, os, os_version,
		       disk_encrypted, disk_encryption_details, antivirus_enabled, antivirus_details,
		       firewall_enabled, firewall_details, screen_lock_enabled, screen_lock_timeout, screen_lock_details,
		       raw_data
		FROM inventory_snapshots
		WHERE machine_id = ?
		ORDER BY collected_at DESC
		LIMIT 1
	`, machineID).Scan(&s.ID, &s.MachineID, &s.CollectedAt, &s.Hostname, &s.OS, &s.OSVersion,
		&s.DiskEncrypted, &s.DiskEncryptionDetails, &s.AntivirusEnabled, &s.AntivirusDetails,
		&firewallEnabled, &firewallDetails, &screenLockEnabled, &screenLockTimeout, &screenLockDetails,
		&s.RawData)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.FirewallEnabled = firewallEnabled.Bool
	s.FirewallDetails = firewallDetails.String
	s.ScreenLockEnabled = screenLockEnabled.Bool
	s.ScreenLockTimeout = int(screenLockTimeout.Int64)
	s.ScreenLockDetails = screenLockDetails.String
	return &s, nil
}

func (db *DB) GetSnapshotHistory(machineID string, limit int) ([]InventorySnapshot, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.conn.Query(`
		SELECT id, machine_id, collected_at, hostname, os, os_version,
		       disk_encrypted, disk_encryption_details, antivirus_enabled, antivirus_details,
		       firewall_enabled, firewall_details, screen_lock_enabled, screen_lock_timeout, screen_lock_details,
		       raw_data
		FROM inventory_snapshots
		WHERE machine_id = ?
		ORDER BY collected_at DESC
		LIMIT ?
	`, machineID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []InventorySnapshot
	for rows.Next() {
		var s InventorySnapshot
		var firewallEnabled, screenLockEnabled sql.NullBool
		var screenLockTimeout sql.NullInt64
		var firewallDetails, screenLockDetails sql.NullString
		if err := rows.Scan(&s.ID, &s.MachineID, &s.CollectedAt, &s.Hostname, &s.OS, &s.OSVersion,
			&s.DiskEncrypted, &s.DiskEncryptionDetails, &s.AntivirusEnabled, &s.AntivirusDetails,
			&firewallEnabled, &firewallDetails, &screenLockEnabled, &screenLockTimeout, &screenLockDetails,
			&s.RawData); err != nil {
			return nil, err
		}
		s.FirewallEnabled = firewallEnabled.Bool
		s.FirewallDetails = firewallDetails.String
		s.ScreenLockEnabled = screenLockEnabled.Bool
		s.ScreenLockTimeout = int(screenLockTimeout.Int64)
		s.ScreenLockDetails = screenLockDetails.String
		snapshots = append(snapshots, s)
	}
	return snapshots, rows.Err()
}

// Dashboard stats
type DashboardStats struct {
	TotalMachines    int
	EncryptedCount   int
	UnencryptedCount int
	ProtectedCount   int
	UnprotectedCount int
	LastChecked      *time.Time
}

func (db *DB) GetUserDashboardStats(userID string) (*DashboardStats, error) {
	stats := &DashboardStats{}

	// Get machine count
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM machines WHERE user_id = ?`, userID).Scan(&stats.TotalMachines)
	if err != nil {
		return nil, err
	}

	// Get encryption stats from latest snapshots
	rows, err := db.conn.Query(`
		SELECT DISTINCT m.id, s.disk_encrypted, s.antivirus_enabled, s.collected_at
		FROM machines m
		LEFT JOIN inventory_snapshots s ON s.id = (
			SELECT id FROM inventory_snapshots WHERE machine_id = m.id ORDER BY collected_at DESC LIMIT 1
		)
		WHERE m.user_id = ?
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var machineID string
		var diskEncrypted, antivirusEnabled sql.NullBool
		var collectedAt sql.NullTime

		if err := rows.Scan(&machineID, &diskEncrypted, &antivirusEnabled, &collectedAt); err != nil {
			return nil, err
		}

		if diskEncrypted.Valid {
			if diskEncrypted.Bool {
				stats.EncryptedCount++
			} else {
				stats.UnencryptedCount++
			}
		}

		if antivirusEnabled.Valid {
			if antivirusEnabled.Bool {
				stats.ProtectedCount++
			} else {
				stats.UnprotectedCount++
			}
		}

		if collectedAt.Valid && (stats.LastChecked == nil || collectedAt.Time.After(*stats.LastChecked)) {
			stats.LastChecked = &collectedAt.Time
		}
	}

	return stats, rows.Err()
}

// Machine notes operations

func (db *DB) CreateMachineNote(machineID, authorID, content string) (*MachineNote, error) {
	result, err := db.conn.Exec(`
		INSERT INTO machine_notes (machine_id, author_id, content) VALUES (?, ?, ?)
	`, machineID, authorID, content)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return db.GetMachineNote(id)
}

func (db *DB) GetMachineNote(id int64) (*MachineNote, error) {
	var n MachineNote
	err := db.conn.QueryRow(`
		SELECT n.id, n.machine_id, n.author_id, u.name, n.content, n.created_at, n.updated_at
		FROM machine_notes n
		JOIN users u ON n.author_id = u.id
		WHERE n.id = ?
	`, id).Scan(&n.ID, &n.MachineID, &n.AuthorID, &n.Author, &n.Content, &n.CreatedAt, &n.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (db *DB) GetMachineNotes(machineID string) ([]MachineNote, error) {
	rows, err := db.conn.Query(`
		SELECT n.id, n.machine_id, n.author_id, u.name, n.content, n.created_at, n.updated_at
		FROM machine_notes n
		JOIN users u ON n.author_id = u.id
		WHERE n.machine_id = ?
		ORDER BY n.created_at DESC
	`, machineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []MachineNote
	for rows.Next() {
		var n MachineNote
		if err := rows.Scan(&n.ID, &n.MachineID, &n.AuthorID, &n.Author, &n.Content, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

func (db *DB) UpdateMachineNote(id int64, content string) error {
	_, err := db.conn.Exec(`
		UPDATE machine_notes SET content = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, content, id)
	return err
}

func (db *DB) DeleteMachineNote(id int64) error {
	_, err := db.conn.Exec(`DELETE FROM machine_notes WHERE id = ?`, id)
	return err
}
