package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHighWaterMark(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() {
		_ = db.Close()
		_ = os.Remove(dbPath)
	}()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Initial value should be 0
	hwm, err := db.GetHighWaterMark()
	if err != nil {
		t.Fatalf("GetHighWaterMark() error: %v", err)
	}
	if hwm != 0 {
		t.Errorf("initial high water mark = %d, want 0", hwm)
	}

	// Set to 100
	if err := db.SetHighWaterMark(100); err != nil {
		t.Fatalf("SetHighWaterMark(100) error: %v", err)
	}

	hwm, err = db.GetHighWaterMark()
	if err != nil {
		t.Fatalf("GetHighWaterMark() error: %v", err)
	}
	if hwm != 100 {
		t.Errorf("high water mark = %d, want 100", hwm)
	}

	// Set to 200 (higher) - should update
	if err := db.SetHighWaterMark(200); err != nil {
		t.Fatalf("SetHighWaterMark(200) error: %v", err)
	}

	hwm, err = db.GetHighWaterMark()
	if err != nil {
		t.Fatalf("GetHighWaterMark() error: %v", err)
	}
	if hwm != 200 {
		t.Errorf("high water mark = %d, want 200", hwm)
	}

	// Set to 150 (lower) - should NOT update
	if err := db.SetHighWaterMark(150); err != nil {
		t.Fatalf("SetHighWaterMark(150) error: %v", err)
	}

	hwm, err = db.GetHighWaterMark()
	if err != nil {
		t.Fatalf("GetHighWaterMark() error: %v", err)
	}
	if hwm != 200 {
		t.Errorf("high water mark = %d, want 200 (should not decrease)", hwm)
	}
}
