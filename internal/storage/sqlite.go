package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

type Service struct {
	ID                 int64  `db:"id"`
	Name               string `db:"name"`
	ContextDescription string `db:"context_description"`
	Status             string `db:"status"`
}

type Photo struct {
	ID        int64  `db:"id"`
	ServiceID int64  `db:"service_id"`
	FilePath  string `db:"file_path"`
	Source    string `db:"source"` // Client, LocalGallery, Stock, AI
	Status    string `db:"status"` // pending, approved, rejected
}

// InitDB opens a connection to the SQLite database and initializes the tables services and photos.
func InitDB(dataSourceName string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign key support
	if _, err := sqlDB.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		sqlDB.Close()
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
	if _, err := sqlDB.Exec(schema); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &DB{db: sqlDB}, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// Ping verifies the database connection.
func (d *DB) Ping() error {
	return d.db.Ping()
}

// CreateService inserts a new service and returns its ID.
func (d *DB) CreateService(ctx context.Context, name string) (int64, error) {
	res, err := d.db.ExecContext(ctx, "INSERT INTO services (name) VALUES (?)", name)
	if err != nil {
		return 0, fmt.Errorf("failed to insert service %s: %w", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID for service: %w", err)
	}
	return id, nil
}

// GetService fetches a service by ID.
func (d *DB) GetService(ctx context.Context, id int64) (*Service, error) {
	row := d.db.QueryRowContext(ctx, "SELECT id, name, COALESCE(context_description, ''), status FROM services WHERE id = ?", id)
	var s Service
	err := row.Scan(&s.ID, &s.Name, &s.ContextDescription, &s.Status)
	if err != nil {
		return nil, fmt.Errorf("failed to scan service %d: %w", id, err)
	}
	return &s, nil
}

// ListServices returns all services.
func (d *DB) ListServices(ctx context.Context) ([]*Service, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT id, name, COALESCE(context_description, ''), status FROM services")
	if err != nil {
		return nil, fmt.Errorf("failed to query services: %w", err)
	}
	defer rows.Close()

	var services []*Service
	for rows.Next() {
		var s Service
		if err := rows.Scan(&s.ID, &s.Name, &s.ContextDescription, &s.Status); err != nil {
			return nil, fmt.Errorf("failed to scan service row: %w", err)
		}
		services = append(services, &s)
	}
	return services, nil
}

// UpdateService updates a service's description and status.
func (d *DB) UpdateService(ctx context.Context, id int64, contextDescription string, status string) error {
	_, err := d.db.ExecContext(ctx, "UPDATE services SET context_description = ?, status = ? WHERE id = ?", contextDescription, status, id)
	if err != nil {
		return fmt.Errorf("failed to update service %d: %w", id, err)
	}
	return nil
}

// CreatePhoto inserts a new photo and returns its ID.
func (d *DB) CreatePhoto(ctx context.Context, serviceID int64, filePath string, source string, status string) (int64, error) {
	res, err := d.db.ExecContext(ctx, "INSERT INTO photos (service_id, file_path, source, status) VALUES (?, ?, ?, ?)", serviceID, filePath, source, status)
	if err != nil {
		return 0, fmt.Errorf("failed to insert photo: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID for photo: %w", err)
	}
	return id, nil
}

// ListPhotosByService returns all photos for a given service.
func (d *DB) ListPhotosByService(ctx context.Context, serviceID int64) ([]*Photo, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT id, service_id, file_path, source, status FROM photos WHERE service_id = ?", serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query photos: %w", err)
	}
	defer rows.Close()

	var photos []*Photo
	for rows.Next() {
		var p Photo
		if err := rows.Scan(&p.ID, &p.ServiceID, &p.FilePath, &p.Source, &p.Status); err != nil {
			return nil, fmt.Errorf("failed to scan photo row: %w", err)
		}
		photos = append(photos, &p)
	}
	return photos, nil
}

// UpdatePhotoStatus updates a photo's status.
func (d *DB) UpdatePhotoStatus(ctx context.Context, id int64, status string) error {
	_, err := d.db.ExecContext(ctx, "UPDATE photos SET status = ? WHERE id = ?", status, id)
	if err != nil {
		return fmt.Errorf("failed to update photo status %d: %w", id, err)
	}
	return nil
}

type ServiceProgress struct {
	ServiceID     int64
	ServiceName   string
	ApprovedCount int
	PendingCount  int
}

// GetServiceProgress returns the approved and pending photo counts for all services.
func (d *DB) GetServiceProgress(ctx context.Context) ([]*ServiceProgress, error) {
	query := `
	SELECT 
		s.id, 
		s.name,
		SUM(CASE WHEN p.status = 'approved' THEN 1 ELSE 0 END) AS approved_count,
		SUM(CASE WHEN p.status = 'pending' THEN 1 ELSE 0 END) AS pending_count
	FROM services s
	LEFT JOIN photos p ON s.id = p.service_id
	GROUP BY s.id, s.name
	`
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query service progress: %w", err)
	}
	defer rows.Close()

	var progress []*ServiceProgress
	for rows.Next() {
		var p ServiceProgress
		if err := rows.Scan(&p.ServiceID, &p.ServiceName, &p.ApprovedCount, &p.PendingCount); err != nil {
			return nil, fmt.Errorf("failed to scan service progress row: %w", err)
		}
		progress = append(progress, &p)
	}
	return progress, nil
}

