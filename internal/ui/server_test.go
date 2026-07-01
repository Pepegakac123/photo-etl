package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pepegakac123/photo-etl/internal/config"
	"github.com/Pepegakac123/photo-etl/internal/storage"
)

func TestWebServer(t *testing.T) {
	// Create mock workspace directory
	tmpDir := t.TempDir()
	clientDir := filepath.Join(tmpDir, "test-client")
	_ = os.Mkdir(clientDir, 0755)

	// Initialize DB
	db, err := storage.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	
	// Create a test service
	serviceID, err := db.CreateService(ctx, "Abbrucharbeiten")
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	cfg := &config.Config{
		TargetPhotosPerService: 5,
		LocalGalleryPath:       tmpDir,
		ExportDir:              filepath.Join(tmpDir, "export"),
	}

	// Initialize Server (passing nil for services we don't need for basic page loads)
	srv := NewServer(db, cfg, nil, nil, nil, clientDir)
	
	// We need to parse templates in the test. Let's make sure our templates are loaded from the relative path
	// or we can test using mock templates, or we can mock/point the templates to the real views directory.
	// Since views/ is in the project root:
	wd, _ := os.Getwd()
	// wd is like /home/kacper/projects/photo-etl/internal/ui
	// the views are in /home/kacper/projects/photo-etl/views
	viewsPath := filepath.Join(wd, "..", "..", "views")
	srv.templatesDir = viewsPath

	err = srv.ParseTemplates()
	if err != nil {
		t.Fatalf("ParseTemplates failed: %v", err)
	}

	// Test GET /
	req, _ := http.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "Overflow Photo ETL") {
		t.Errorf("expected response to contain application title, got: %s", rr.Body.String())
	}

	// Test GET /services/{id}
	req, _ = http.NewRequest("GET", "/services/1", nil)
	// Mock URL routing inside handler since we are calling ServeHTTP directly
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 for service, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "Abbrucharbeiten") {
		t.Errorf("expected response to contain service name, got: %s", rr.Body.String())
	}

	// Test POST /photos/{id}/approve
	// Let's insert a photo in the DB
	_, err = db.CreatePhoto(ctx, serviceID, "/path/to/img.jpg", "Client", "pending")
	if err != nil {
		t.Fatalf("failed to create photo: %v", err)
	}

	req, _ = http.NewRequest("POST", "/photos/1/approve", nil)
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// Verify photo is approved in DB
	photos, err := db.ListPhotosByService(ctx, serviceID)
	if err != nil {
		t.Fatalf("failed to list photos: %v", err)
	}
	if len(photos) != 1 || photos[0].Status != "approved" {
		t.Errorf("expected photo status to be approved, got %+v", photos)
	}

	if !strings.Contains(rr.Body.String(), "hx-swap-oob") {
		t.Errorf("expected response to include OOB counter updates, got: %s", rr.Body.String())
	}
}
