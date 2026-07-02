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

type GalleryFolder struct {
	ID         int64  `db:"id"`
	FolderName string `db:"folder_name"`
	FolderPath string `db:"folder_path"`
	GermanName string `db:"german_name"`
	PolishName string `db:"polish_name"`
}

// InitDB opens a connection to the SQLite database and initializes the tables services, photos, and gallery_folders.
func InitDB(dataSourceName string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if dataSourceName == ":memory:" {
		sqlDB.SetMaxOpenConns(1)
	}

	// Enable foreign key support, WAL mode, and busy timeout
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, pragma := range pragmas {
		if _, err := sqlDB.Exec(pragma); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("failed to execute pragma %q: %w", pragma, err)
		}
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

	CREATE TABLE IF NOT EXISTS gallery_folders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		folder_name TEXT UNIQUE NOT NULL,
		folder_path TEXT NOT NULL,
		german_name TEXT NOT NULL,
		polish_name TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS api_costs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		service_name TEXT NOT NULL,
		operation_type TEXT NOT NULL,
		model_used TEXT NOT NULL,
		prompt_tokens INTEGER DEFAULT 0,
		completion_tokens INTEGER DEFAULT 0,
		calculated_cost REAL NOT NULL DEFAULT 0.0,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
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

// CreateGalleryFolder inserts a new gallery folder and returns its ID.
func (d *DB) CreateGalleryFolder(ctx context.Context, folderName, folderPath, germanName, polishName string) (int64, error) {
	res, err := d.db.ExecContext(ctx, "INSERT OR IGNORE INTO gallery_folders (folder_name, folder_path, german_name, polish_name) VALUES (?, ?, ?, ?)", folderName, folderPath, germanName, polishName)
	if err != nil {
		return 0, fmt.Errorf("failed to insert gallery folder %s: %w", folderName, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID for gallery folder: %w", err)
	}
	// If INSERT OR IGNORE ignored it, res.LastInsertId() might be 0. Look it up.
	if id == 0 {
		var existingID int64
		err := d.db.QueryRowContext(ctx, "SELECT id FROM gallery_folders WHERE folder_name = ?", folderName).Scan(&existingID)
		if err == nil {
			return existingID, nil
		}
	}
	return id, nil
}

// ListGalleryFolders returns all indexed gallery folders.
func (d *DB) ListGalleryFolders(ctx context.Context) ([]*GalleryFolder, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT id, folder_name, folder_path, german_name, polish_name FROM gallery_folders")
	if err != nil {
		return nil, fmt.Errorf("failed to query gallery folders: %w", err)
	}
	defer rows.Close()

	var folders []*GalleryFolder
	for rows.Next() {
		var f GalleryFolder
		if err := rows.Scan(&f.ID, &f.FolderName, &f.FolderPath, &f.GermanName, &f.PolishName); err != nil {
			return nil, fmt.Errorf("failed to scan gallery folder row: %w", err)
		}
		folders = append(folders, &f)
	}
	return folders, nil
}

// GetPhoto retrieves a single photo by its ID.
func (d *DB) GetPhoto(ctx context.Context, id int64) (*Photo, error) {
	row := d.db.QueryRowContext(ctx, "SELECT id, service_id, file_path, source, status FROM photos WHERE id = ?", id)
	var p Photo
	err := row.Scan(&p.ID, &p.ServiceID, &p.FilePath, &p.Source, &p.Status)
	if err != nil {
		return nil, fmt.Errorf("failed to scan photo %d: %w", id, err)
	}
	return &p, nil
}

type APICost struct {
	ID               int64   `db:"id"`
	ServiceName      string  `db:"service_name"`
	OperationType    string  `db:"operation_type"`
	ModelUsed        string  `db:"model_used"`
	PromptTokens     int     `db:"prompt_tokens"`
	CompletionTokens int     `db:"completion_tokens"`
	CalculatedCost   float64 `db:"calculated_cost"`
	Timestamp        string  `db:"timestamp"`
}

// LogCost inserts an API call log and its cost.
func (d *DB) LogCost(ctx context.Context, serviceName, operationType, modelUsed string, promptTokens, completionTokens int, calculatedCost float64) error {
	_, err := d.db.ExecContext(ctx, `
		INSERT INTO api_costs (service_name, operation_type, model_used, prompt_tokens, completion_tokens, calculated_cost)
		VALUES (?, ?, ?, ?, ?, ?)
	`, serviceName, operationType, modelUsed, promptTokens, completionTokens, calculatedCost)
	if err != nil {
		return fmt.Errorf("failed to log API cost: %w", err)
	}
	return nil
}

// GetTotalCosts returns the sum of all logged API costs.
func (d *DB) GetTotalCosts(ctx context.Context) (float64, error) {
	var total float64
	err := d.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(calculated_cost), 0.0) FROM api_costs").Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get total costs: %w", err)
	}
	return total, nil
}

// ListCosts returns all logged API costs.
func (d *DB) ListCosts(ctx context.Context) ([]*APICost, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT id, service_name, operation_type, model_used, prompt_tokens, completion_tokens, calculated_cost, strftime('%Y-%m-%d %H:%M:%S', timestamp) FROM api_costs ORDER BY id DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to query api costs: %w", err)
	}
	defer rows.Close()

	var costs []*APICost
	for rows.Next() {
		var c APICost
		if err := rows.Scan(&c.ID, &c.ServiceName, &c.OperationType, &c.ModelUsed, &c.PromptTokens, &c.CompletionTokens, &c.CalculatedCost, &c.Timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan api cost row: %w", err)
		}
		costs = append(costs, &c)
	}
	return costs, nil
}

// ClearCosts clears all logged API costs.
func (d *DB) ClearCosts(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, "DELETE FROM api_costs")
	if err != nil {
		return fmt.Errorf("failed to clear api costs: %w", err)
	}
	return nil
}

// PhotoExistsForService checks if a photo with the given filePath is already registered under the service.
func (d *DB) PhotoExistsForService(ctx context.Context, serviceID int64, filePath string) (bool, error) {
	var count int
	err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM photos WHERE service_id = ? AND file_path = ?", serviceID, filePath).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetActivePhotoPaths returns all file paths of photos that are approved or pending in DB.
func (d *DB) GetActivePhotoPaths(ctx context.Context) (map[string]bool, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT file_path FROM photos WHERE status IN ('approved', 'pending')")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := make(map[string]bool)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err == nil {
			paths[path] = true
		}
	}
	return paths, nil
}

// AddOrApprovePhoto inserts a photo or updates its status to 'approved' if it was rejected.
func (d *DB) AddOrApprovePhoto(ctx context.Context, serviceID int64, filePath string, source string) error {
	var id int64
	var status string
	err := d.db.QueryRowContext(ctx, "SELECT id, status FROM photos WHERE service_id = ? AND file_path = ?", serviceID, filePath).Scan(&id, &status)
	if err == nil {
		// Record exists, update status to approved
		_, err = d.db.ExecContext(ctx, "UPDATE photos SET status = 'approved' WHERE id = ?", id)
		return err
	}

	// Record does not exist, insert new
	_, err = d.db.ExecContext(ctx, "INSERT INTO photos (service_id, file_path, source, status) VALUES (?, ?, ?, 'approved')", serviceID, filePath, source)
	return err
}

// GetGalleryFolder retrieves a gallery folder by its ID.
func (d *DB) GetGalleryFolder(ctx context.Context, id int64) (*GalleryFolder, error) {
	var f GalleryFolder
	err := d.db.QueryRowContext(ctx, "SELECT id, folder_name, folder_path, german_name, polish_name FROM gallery_folders WHERE id = ?", id).Scan(
		&f.ID, &f.FolderName, &f.FolderPath, &f.GermanName, &f.PolishName,
	)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// UpdateServiceContextDescription updates a service's context description.
func (d *DB) UpdateServiceContextDescription(ctx context.Context, id int64, desc string) error {
	_, err := d.db.ExecContext(ctx, "UPDATE services SET context_description = ? WHERE id = ?", desc, id)
	return err
}

// GetAllPhotoPaths returns all file paths of photos in DB.
func (d *DB) GetAllPhotoPaths(ctx context.Context) (map[string]bool, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT file_path FROM photos")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := make(map[string]bool)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err == nil {
			paths[path] = true
		}
	}
	return paths, nil
}



