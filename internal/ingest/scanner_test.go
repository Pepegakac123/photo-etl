package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Pepegakac123/photo-etl/internal/storage"
)

func TestScanner(t *testing.T) {
	// Create temporary client directory structure
	tmpDir, err := os.MkdirTemp("", "client-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create service directories and files
	abbruchDir := filepath.Join(tmpDir, "Abbrucharbeiten")
	if err := os.Mkdir(abbruchDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(abbruchDir, "photo1.jpg"), []byte("fake-jpeg"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(abbruchDir, "photo2.png"), []byte("fake-png"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(abbruchDir, "notes.txt"), []byte("text data"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	fassadenDir := filepath.Join(tmpDir, "Fassadenbau")
	if err := os.Mkdir(fassadenDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fassadenDir, "photo3.jpeg"), []byte("fake-jpeg"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	whatsappDir := filepath.Join(tmpDir, "Zrzuty-whatsapp")
	if err := os.Mkdir(whatsappDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	// Initialize DB
	db, err := storage.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	scanner := NewScanner(db)
	res, err := scanner.Scan(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if res.ScreenshotsFolder != whatsappDir {
		t.Errorf("expected screenshots folder %q, got %q", whatsappDir, res.ScreenshotsFolder)
	}

	expectedServices := map[string]bool{
		"Abbrucharbeiten": true,
		"Fassadenbau":     true,
	}
	if len(res.ServicesAdded) != 2 {
		t.Errorf("expected 2 services added, got %d", len(res.ServicesAdded))
	}
	for _, sName := range res.ServicesAdded {
		if !expectedServices[sName] {
			t.Errorf("unexpected service added: %s", sName)
		}
	}

	// Retrieve services from DB to check integration
	services, err := db.ListServices(ctx)
	if err != nil {
		t.Fatalf("failed to list services: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("expected 2 services in DB, got %d", len(services))
	}

	for _, s := range services {
		photos, err := db.ListPhotosByService(ctx, s.ID)
		if err != nil {
			t.Fatalf("failed to list photos for %s: %v", s.Name, err)
		}
		if s.Name == "Abbrucharbeiten" {
			if len(photos) != 2 {
				t.Errorf("expected 2 photos for Abbrucharbeiten, got %d", len(photos))
			}
			for _, p := range photos {
				if p.Source != "Client" || p.Status != "approved" {
					t.Errorf("unexpected photo attributes: %+v", p)
				}
				if filepath.Base(p.FilePath) == "notes.txt" {
					t.Errorf("notes.txt should have been ignored")
				}
			}
		} else if s.Name == "Fassadenbau" {
			if len(photos) != 1 {
				t.Errorf("expected 1 photo for Fassadenbau, got %d", len(photos))
			}
		}
	}
}

// Helper to reuse in other packages or tests if needed
func IsImage(filename string) bool {
	ext := filepath.Ext(filename)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	}
	return false
}
