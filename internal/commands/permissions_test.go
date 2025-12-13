package commands

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// Test keypairs (generated for testing, not real keys)
const (
	adminPubkeyHex = "0000000000000000000000000000000000000000000000000000000000000001"
	adminNpub      = "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqshp52w2"

	customerPubkeyHex = "0000000000000000000000000000000000000000000000000000000000000002"
	customerNpub      = "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqpqdangsl"

	unknownPubkeyHex = "0000000000000000000000000000000000000000000000000000000000000003"
)

func TestIsAdmin(t *testing.T) {
	admins := []string{adminNpub}

	tests := []struct {
		name       string
		pubkeyHex  string
		admins     []string
		wantResult bool
	}{
		{
			name:       "admin pubkey returns true",
			pubkeyHex:  adminPubkeyHex,
			admins:     admins,
			wantResult: true,
		},
		{
			name:       "non-admin pubkey returns false",
			pubkeyHex:  customerPubkeyHex,
			admins:     admins,
			wantResult: false,
		},
		{
			name:       "empty admin list returns false",
			pubkeyHex:  adminPubkeyHex,
			admins:     []string{},
			wantResult: false,
		},
		{
			name:       "invalid hex pubkey returns false",
			pubkeyHex:  "notahexpubkey",
			admins:     admins,
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAdmin(tt.pubkeyHex, tt.admins)
			if got != tt.wantResult {
				t.Errorf("IsAdmin(%q, %v) = %v, want %v", tt.pubkeyHex, tt.admins, got, tt.wantResult)
			}
		})
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}

	// Create customers table
	_, err = db.Exec(`
		CREATE TABLE customers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			npub TEXT NOT NULL UNIQUE,
			name TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("creating customers table: %v", err)
	}

	// Insert test customer
	_, err = db.Exec("INSERT INTO customers (npub) VALUES (?)", customerNpub)
	if err != nil {
		t.Fatalf("inserting test customer: %v", err)
	}

	return db
}

func TestIsCustomer(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	tests := []struct {
		name       string
		pubkeyHex  string
		wantResult bool
		wantErr    bool
	}{
		{
			name:       "registered customer returns true",
			pubkeyHex:  customerPubkeyHex,
			wantResult: true,
			wantErr:    false,
		},
		{
			name:       "unknown user returns false",
			pubkeyHex:  unknownPubkeyHex,
			wantResult: false,
			wantErr:    false,
		},
		{
			name:       "admin (not in customers table) returns false",
			pubkeyHex:  adminPubkeyHex,
			wantResult: false,
			wantErr:    false,
		},
		{
			name:       "invalid hex pubkey returns error",
			pubkeyHex:  "notahexpubkey",
			wantResult: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsCustomer(ctx, db, tt.pubkeyHex)

			if tt.wantErr {
				if err == nil {
					t.Errorf("IsCustomer(%q) error = nil, want error", tt.pubkeyHex)
				}
				return
			}

			if err != nil {
				t.Errorf("IsCustomer(%q) error = %v, want nil", tt.pubkeyHex, err)
				return
			}

			if got != tt.wantResult {
				t.Errorf("IsCustomer(%q) = %v, want %v", tt.pubkeyHex, got, tt.wantResult)
			}
		})
	}
}

func TestCanExecute(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	admins := []string{adminNpub}

	tests := []struct {
		name      string
		cmd       *Command
		pubkeyHex string
		wantErr   bool
	}{
		{
			name:      "admin can execute customer command",
			cmd:       &Command{Name: CmdInventory},
			pubkeyHex: adminPubkeyHex,
			wantErr:   false,
		},
		{
			name:      "admin can execute admin command",
			cmd:       &Command{Name: CmdAdd},
			pubkeyHex: adminPubkeyHex,
			wantErr:   false,
		},
		{
			name:      "customer can execute customer command",
			cmd:       &Command{Name: CmdInventory},
			pubkeyHex: customerPubkeyHex,
			wantErr:   false,
		},
		{
			name:      "customer cannot execute admin command",
			cmd:       &Command{Name: CmdAdd},
			pubkeyHex: customerPubkeyHex,
			wantErr:   true,
		},
		{
			name:      "unknown user cannot execute customer command",
			cmd:       &Command{Name: CmdInventory},
			pubkeyHex: unknownPubkeyHex,
			wantErr:   true,
		},
		{
			name:      "unknown user cannot execute admin command",
			cmd:       &Command{Name: CmdAdd},
			pubkeyHex: unknownPubkeyHex,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CanExecute(ctx, db, tt.cmd, tt.pubkeyHex, admins)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CanExecute(%v, %q) = nil, want error", tt.cmd.Name, tt.pubkeyHex)
				}
			} else {
				if err != nil {
					t.Errorf("CanExecute(%v, %q) = %v, want nil", tt.cmd.Name, tt.pubkeyHex, err)
				}
			}
		})
	}
}
