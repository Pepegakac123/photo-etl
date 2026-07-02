package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fmt"
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
	srv := NewServer(db, cfg, "", nil, nil, nil, clientDir)
	
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

func TestAssociateFolder(t *testing.T) {
	tmpDir := t.TempDir()
	clientDir := filepath.Join(tmpDir, "test-client")
	_ = os.Mkdir(clientDir, 0755)

	db, err := storage.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.CreateService(ctx, "Abbrucharbeiten")
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Create a mock gallery folder
	mockFolderPath := filepath.Join(tmpDir, "mock-gallery-folder")
	_ = os.MkdirAll(mockFolderPath, 0755)
	// Write a mock image file
	_ = os.WriteFile(filepath.Join(mockFolderPath, "image1.jpg"), []byte("fake"), 0644)

	folderID, err := db.CreateGalleryFolder(ctx, "mock-gallery-folder", mockFolderPath, "mock-de", "mock-pl")
	if err != nil {
		t.Fatalf("failed to create gallery folder: %v", err)
	}

	cfg := &config.Config{
		TargetPhotosPerService: 5,
		LocalGalleryPath:       tmpDir,
		ExportDir:              filepath.Join(tmpDir, "export"),
	}

	srv := NewServer(db, cfg, "", nil, nil, nil, clientDir)
	wd, _ := os.Getwd()
	srv.templatesDir = filepath.Join(wd, "..", "..", "views")
	_ = srv.ParseTemplates()

	// Test POST /services/1/gallery/associate-folder with folder_id in form urlencoded body
	req, _ := http.NewRequest("POST", "/services/1/gallery/associate-folder", strings.NewReader(fmt.Sprintf("folder_id=%d", folderID)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Set path value since we bypass mux routing
	req.SetPathValue("id", "1")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	t.Logf("Response code: %d", rr.Code)
	t.Logf("Response body: %s", rr.Body.String())

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	if !strings.Contains(rr.Body.String(), "mock-gallery-folder") {
		t.Errorf("expected response to contain folder name 'mock-gallery-folder', got: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "image1.jpg") {
		t.Errorf("expected response to contain image filename 'image1.jpg', got: %s", rr.Body.String())
	}
}

func TestExportWithWhatsAppFolder(t *testing.T) {
	tmpDir := t.TempDir()
	clientDir := filepath.Join(tmpDir, "test-client")
	_ = os.Mkdir(clientDir, 0755)

	// Create WhatsApp directory in the client directory
	whatsappDir := filepath.Join(clientDir, "whatsapp-photos")
	_ = os.Mkdir(whatsappDir, 0755)

	// Write mock images to WhatsApp directory
	_ = os.WriteFile(filepath.Join(whatsappDir, "photo1.jpg"), []byte("photo1"), 0644)
	_ = os.WriteFile(filepath.Join(whatsappDir, "photo2.jpg"), []byte("photo2"), 0644)
	_ = os.WriteFile(filepath.Join(whatsappDir, "photo3.jpg"), []byte("photo3"), 0644)
	_ = os.WriteFile(filepath.Join(whatsappDir, "not-image.txt"), []byte("text"), 0644)

	db, err := storage.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	serviceID, err := db.CreateService(ctx, "ServiceA")
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Create 5 approved photos for ServiceA so that export minimum limit is satisfied
	for i := 1; i <= 5; i++ {
		photoPath := filepath.Join(tmpDir, fmt.Sprintf("service_photo%d.jpg", i))
		_ = os.WriteFile(photoPath, []byte("service_photo"), 0644)
		_, err = db.CreatePhoto(ctx, serviceID, photoPath, "LocalGallery", "approved")
		if err != nil {
			t.Fatalf("failed to create photo: %v", err)
		}
	}

	// Mark photo3 as rejected in the DB
	rejectedPath := filepath.Join(whatsappDir, "photo3.jpg")
	_, err = db.CreatePhoto(ctx, serviceID, rejectedPath, "Client", "rejected")
	if err != nil {
		t.Fatalf("failed to create rejected photo in DB: %v", err)
	}

	cfg := &config.Config{
		TargetPhotosPerService: 5,
		LocalGalleryPath:       tmpDir,
		ExportDir:              filepath.Join(tmpDir, "export"),
	}

	srv := NewServer(db, cfg, "", nil, nil, nil, clientDir)
	wd, _ := os.Getwd()
	srv.templatesDir = filepath.Join(wd, "..", "..", "views")
	_ = srv.ParseTemplates()

	// Perform export
	req, _ := http.NewRequest("POST", "/export", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected export status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	// Verify files in exportDir/whatsapp-photos
	exportWhatsappDir := filepath.Join(cfg.ExportDir, "whatsapp-photos")
	
	// Should contain photo1.jpg and photo2.jpg
	if _, err := os.Stat(filepath.Join(exportWhatsappDir, "photo1.jpg")); err != nil {
		t.Errorf("expected photo1.jpg to be exported, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(exportWhatsappDir, "photo2.jpg")); err != nil {
		t.Errorf("expected photo2.jpg to be exported, got error: %v", err)
	}

	// Should NOT contain photo3.jpg (rejected)
	if _, err := os.Stat(filepath.Join(exportWhatsappDir, "photo3.jpg")); !os.IsNotExist(err) {
		t.Errorf("expected photo3.jpg (rejected) to not be exported, but it exists")
	}

	// Should NOT contain not-image.txt (filtered by scanIsImage)
	if _, err := os.Stat(filepath.Join(exportWhatsappDir, "not-image.txt")); !os.IsNotExist(err) {
		t.Errorf("expected not-image.txt (non-image) to not be exported, but it exists")
	}
}

