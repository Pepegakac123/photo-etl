package storage

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestInitDB(t *testing.T) {
	// Initialize in-memory database
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer db.Close()

	// Verify 'services' table exists and has correct columns
	checkTableColumns(t, db, "services", map[string]string{
		"id":                  "INTEGER",
		"name":                "TEXT",
		"context_description": "TEXT",
		"status":              "TEXT",
	})

	// Verify 'photos' table exists and has correct columns
	checkTableColumns(t, db, "photos", map[string]string{
		"id":         "INTEGER",
		"service_id": "INTEGER",
		"file_path":  "TEXT",
		"source":     "TEXT",
		"status":     "TEXT",
	})
}

func checkTableColumns(t *testing.T, db *sql.DB, tableName string, expectedCols map[string]string) {
	t.Helper()

	// Check table existence
	var count int
	err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&count)
	if err != nil {
		t.Fatalf("failed to check table existence for %s: %v", tableName, err)
	}
	if count != 1 {
		t.Fatalf("table %s does not exist", tableName)
	}

	// Check table columns using PRAGMA table_info
	rows, err := db.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		t.Fatalf("failed to query table info for %s: %v", tableName, err)
	}
	defer rows.Close()

	actualCols := make(map[string]string)
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("failed to scan table info for %s: %v", tableName, err)
		}
		actualCols[name] = ctype
	}

	for name, expectedType := range expectedCols {
		actualType, ok := actualCols[name]
		if !ok {
			t.Errorf("table %s is missing expected column %q", tableName, name)
			continue
		}
		// In SQLite, type check might be case-insensitive or allow variants, but ours will match exactly.
		if actualType != expectedType {
			t.Errorf("column %s.%s has type %q, expected %q", tableName, name, actualType, expectedType)
		}
	}
}
