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
	checkTableColumns(t, db.db, "services", map[string]string{
		"id":                  "INTEGER",
		"name":                "TEXT",
		"context_description": "TEXT",
		"status":              "TEXT",
	})

	// Verify 'photos' table exists and has correct columns
	checkTableColumns(t, db.db, "photos", map[string]string{
		"id":         "INTEGER",
		"service_id": "INTEGER",
		"file_path":  "TEXT",
		"source":     "TEXT",
		"status":     "TEXT",
	})
}

func TestStorageOperations(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer db.Close()

	ctx := t.Context() // Go 1.22+ Context for tests

	// 1. Create a service
	serviceID, err := db.CreateService(ctx, "Abbrucharbeiten")
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	if serviceID == 0 {
		t.Fatal("expected non-zero service ID")
	}

	// 2. Get the service and verify
	svc, err := db.GetService(ctx, serviceID)
	if err != nil {
		t.Fatalf("failed to get service: %v", err)
	}
	if svc.Name != "Abbrucharbeiten" || svc.Status != "pending" || svc.ContextDescription != "" {
		t.Errorf("unexpected service values: %+v", svc)
	}

	// 3. Update service
	err = db.UpdateService(ctx, serviceID, "Demolition works in German", "completed")
	if err != nil {
		t.Fatalf("failed to update service: %v", err)
	}
	svc, err = db.GetService(ctx, serviceID)
	if err != nil {
		t.Fatalf("failed to get service after update: %v", err)
	}
	if svc.Status != "completed" || svc.ContextDescription != "Demolition works in German" {
		t.Errorf("expected updated values, got %+v", svc)
	}

	// 4. Create photos
	p1ID, err := db.CreatePhoto(ctx, serviceID, "/path/to/img1.jpg", "Client", "approved")
	if err != nil {
		t.Fatalf("failed to create photo 1: %v", err)
	}
	p2ID, err := db.CreatePhoto(ctx, serviceID, "/path/to/img2.jpg", "Stock", "pending")
	if err != nil {
		t.Fatalf("failed to create photo 2: %v", err)
	}

	// 5. List photos and verify
	photos, err := db.ListPhotosByService(ctx, serviceID)
	if err != nil {
		t.Fatalf("failed to list photos: %v", err)
	}
	if len(photos) != 2 {
		t.Fatalf("expected 2 photos, got %d", len(photos))
	}
	if photos[0].ID != p1ID || photos[0].Source != "Client" || photos[0].Status != "approved" {
		t.Errorf("unexpected values for photo 1: %+v", photos[0])
	}

	// 6. Update photo status
	err = db.UpdatePhotoStatus(ctx, p2ID, "approved")
	if err != nil {
		t.Fatalf("failed to update photo status: %v", err)
	}
	photos, err = db.ListPhotosByService(ctx, serviceID)
	if err != nil {
		t.Fatalf("failed to list photos: %v", err)
	}
	if photos[1].Status != "approved" {
		t.Errorf("expected photo 2 status to be approved, got %s", photos[1].Status)
	}

	// 7. Verify ListServices
	services, err := db.ListServices(ctx)
	if err != nil {
		t.Fatalf("failed to list services: %v", err)
	}
	if len(services) != 1 || services[0].ID != serviceID {
		t.Errorf("expected 1 service with ID %d, got %v", serviceID, services)
	}

	// 8. Verify Foreign Key constraint (delete service cascades to photos)
	_, err = db.db.Exec("DELETE FROM services WHERE id = ?", serviceID)
	if err != nil {
		t.Fatalf("failed to delete service: %v", err)
	}
	photos, err = db.ListPhotosByService(ctx, serviceID)
	if err != nil {
		t.Fatalf("failed to list photos after cascade delete: %v", err)
	}
	if len(photos) != 0 {
		t.Errorf("expected 0 photos due to cascade, got %d", len(photos))
	}
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

func TestServiceProgress(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer db.Close()

	ctx := t.Context()

	// 1. Create services
	s1ID, err := db.CreateService(ctx, "Abbrucharbeiten")
	if err != nil {
		t.Fatalf("failed to create service 1: %v", err)
	}
	s2ID, err := db.CreateService(ctx, "Fassadenbau")
	if err != nil {
		t.Fatalf("failed to create service 2: %v", err)
	}

	// 2. Insert photos for s1
	_, _ = db.CreatePhoto(ctx, s1ID, "/path/1", "Client", "approved")
	_, _ = db.CreatePhoto(ctx, s1ID, "/path/2", "Client", "approved")
	_, _ = db.CreatePhoto(ctx, s1ID, "/path/3", "Client", "pending")
	_, _ = db.CreatePhoto(ctx, s1ID, "/path/4", "Client", "rejected")

	// 3. Insert photos for s2 (none)

	// 4. Query progress
	progress, err := db.GetServiceProgress(ctx)
	if err != nil {
		t.Fatalf("failed to get service progress: %v", err)
	}

	if len(progress) != 2 {
		t.Fatalf("expected progress for 2 services, got %d", len(progress))
	}

	var p1, p2 *ServiceProgress
	for _, p := range progress {
		if p.ServiceID == s1ID {
			p1 = p
		} else if p.ServiceID == s2ID {
			p2 = p
		}
	}

	if p1 == nil || p2 == nil {
		t.Fatal("expected both services in progress result")
	}

	if p1.ApprovedCount != 2 {
		t.Errorf("expected 2 approved for Abbrucharbeiten, got %d", p1.ApprovedCount)
	}
	if p1.PendingCount != 1 {
		t.Errorf("expected 1 pending for Abbrucharbeiten, got %d", p1.PendingCount)
	}

	if p2.ApprovedCount != 0 {
		t.Errorf("expected 0 approved for Fassadenbau, got %d", p2.ApprovedCount)
	}
	if p2.PendingCount != 0 {
		t.Errorf("expected 0 pending for Fassadenbau, got %d", p2.PendingCount)
	}
}

func TestGalleryFolderOperations(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer db.Close()

	ctx := t.Context()

	// 1. Check schema (table existence)
	checkTableColumns(t, db.db, "gallery_folders", map[string]string{
		"id":          "INTEGER",
		"folder_name": "TEXT",
		"folder_path": "TEXT",
		"german_name": "TEXT",
		"polish_name": "TEXT",
	})

	// 2. Insert gallery folders
	id, err := db.CreateGalleryFolder(ctx, "Badsanierung_Remont łazienki", "/path/to/bad", "Badsanierung", "Remont łazienki")
	if err != nil {
		t.Fatalf("failed to create gallery folder: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero ID")
	}

	// 3. List gallery folders
	folders, err := db.ListGalleryFolders(ctx)
	if err != nil {
		t.Fatalf("failed to list gallery folders: %v", err)
	}
	if len(folders) != 1 {
		t.Fatalf("expected 1 folder, got %d", len(folders))
	}
	f := folders[0]
	if f.FolderName != "Badsanierung_Remont łazienki" || f.GermanName != "Badsanierung" || f.PolishName != "Remont łazienki" {
		t.Errorf("unexpected folder attributes: %+v", f)
	}
}

