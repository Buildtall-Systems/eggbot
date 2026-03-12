package commands

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// Generated with: nak key generate && nak key public && nak encode npub
const (
	adminNpub    = "npub10r67600hzrjqfjprydg0a25u9vry9zkjn8ukf9ynh9y2whlxrjesxdl8rn"
	customerNpub = "npub1cccr2442yt4qkuejczqc9tzuxzfq8gtvjkzf8q3lvx5eyn8rjacq3mnvjx"
	unknownNpub  = "npub1lqq6patcatq5nkrk5qfqest6qpu4x6xhg0ydly5j9x4sxhxj78eq4c9kxq"
)

func TestIsAdmin(t *testing.T) {
	admins := []string{adminNpub}

	tests := []struct {
		name       string
		npub       string
		admins     []string
		wantResult bool
	}{
		{
			name:       "admin npub returns true",
			npub:       adminNpub,
			admins:     admins,
			wantResult: true,
		},
		{
			name:       "non-admin npub returns false",
			npub:       customerNpub,
			admins:     admins,
			wantResult: false,
		},
		{
			name:       "empty admin list returns false",
			npub:       adminNpub,
			admins:     []string{},
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAdmin(tt.npub, tt.admins)
			if got != tt.wantResult {
				t.Errorf("IsAdmin(%q, %v) = %v, want %v", tt.npub, tt.admins, got, tt.wantResult)
			}
		})
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}

	// Create customers table
	_, err = db.ExecContext(ctx, `
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
	_, err = db.ExecContext(ctx, "INSERT INTO customers (npub) VALUES (?)", customerNpub)
	if err != nil {
		t.Fatalf("inserting test customer: %v", err)
	}

	return db
}

func TestIsCustomer(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("closing test db: %v", err)
		}
	})

	ctx := context.Background()
	admins := []string{adminNpub}

	tests := []struct {
		name       string
		npub       string
		admins     []string
		wantResult bool
	}{
		{
			name:       "registered customer returns true",
			npub:       customerNpub,
			admins:     admins,
			wantResult: true,
		},
		{
			name:       "unknown user returns false",
			npub:       unknownNpub,
			admins:     admins,
			wantResult: false,
		},
		{
			name:       "admin (not in customers table) returns true (implicit customer)",
			npub:       adminNpub,
			admins:     admins,
			wantResult: true,
		},
		{
			name:       "admin with empty admin list returns false",
			npub:       adminNpub,
			admins:     []string{},
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsCustomer(ctx, db, tt.npub, tt.admins)
			if err != nil {
				t.Errorf("IsCustomer(%q) error = %v, want nil", tt.npub, err)
				return
			}

			if got != tt.wantResult {
				t.Errorf("IsCustomer(%q) = %v, want %v", tt.npub, got, tt.wantResult)
			}
		})
	}
}

func TestCanExecute(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("closing test db: %v", err)
		}
	})

	ctx := context.Background()
	admins := []string{adminNpub}

	tests := []struct {
		name    string
		cmd     *Command
		npub    string
		wantErr bool
	}{
		{
			name:    "admin can execute customer command",
			cmd:     &Command{Name: CmdInventory},
			npub:    adminNpub,
			wantErr: false,
		},
		{
			name:    "admin can execute admin command",
			cmd:     &Command{Name: CmdDeliver},
			npub:    adminNpub,
			wantErr: false,
		},
		{
			name:    "customer can execute customer command",
			cmd:     &Command{Name: CmdInventory},
			npub:    customerNpub,
			wantErr: false,
		},
		{
			name:    "customer cannot execute admin command",
			cmd:     &Command{Name: CmdDeliver},
			npub:    customerNpub,
			wantErr: true,
		},
		{
			name:    "unknown user cannot execute customer command",
			cmd:     &Command{Name: CmdInventory},
			npub:    unknownNpub,
			wantErr: true,
		},
		{
			name:    "unknown user cannot execute admin command",
			cmd:     &Command{Name: CmdDeliver},
			npub:    unknownNpub,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CanExecute(ctx, db, tt.cmd, tt.npub, admins)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CanExecute(%v, %q) = nil, want error", tt.cmd.Name, tt.npub)
				}
			} else {
				if err != nil {
					t.Errorf("CanExecute(%v, %q) = %v, want nil", tt.cmd.Name, tt.npub, err)
				}
			}
		})
	}
}
