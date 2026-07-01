package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// InitDB opens a connection to the SQLite database and initializes the tables services and photos.
func InitDB(dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign key support
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS services (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		context_description TEXT,
		status TEXT NOT NULL DEFAULT 'pending'
	);

	CREATE TABLE IF NOT EXISTS photos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		service_id INTEGER NOT NULL,
		file_path TEXT NOT NULL,
		source TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		FOREIGN KEY(service_id) REFERENCES services(id) ON DELETE CASCADE
	);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}
