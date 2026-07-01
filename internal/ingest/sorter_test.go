package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Pepegakac123/photo-etl/internal/storage"
)

type mockClassifier struct {
	responses map[string]string
}

func (m *mockClassifier) ClassifyImage(ctx context.Context, imagePath string, categories []string) (string, error) {
	filename := filepath.Base(imagePath)
	if resp, ok := m.responses[filename]; ok {
		return resp, nil
	}
	return "REJECT", nil
}

func TestSorter(t *testing.T) {
	// Create temporary screenshots directory and files
	tmpDir, err := os.MkdirTemp("", "screenshots-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	img1Path := filepath.Join(tmpDir, "img1.jpg")
	img2Path := filepath.Join(tmpDir, "img2.png")
	img3Path := filepath.Join(tmpDir, "notes.txt") // non-image, should be ignored

	if err := os.WriteFile(img1Path, []byte("fake-img-1"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := os.WriteFile(img2Path, []byte("fake-img-2"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := os.WriteFile(img3Path, []byte("some text"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Initialize DB
	db, err := storage.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Insert services
	s1ID, err := db.CreateService(ctx, "Abbrucharbeiten")
	if err != nil {
		t.Fatalf("failed to create service 1: %v", err)
	}
	_, err = db.CreateService(ctx, "Fassadenbau")
	if err != nil {
		t.Fatalf("failed to create service 2: %v", err)
	}

	// Create mock classifier: img1 matches "Abbrucharbeiten", img2 is "REJECT"
	mc := &mockClassifier{
		responses: map[string]string{
			"img1.jpg": "Abbrucharbeiten",
			"img2.png": "REJECT",
		},
	}

	sorter := NewSorter(db, mc, 2)
	err = sorter.SortScreenshots(ctx, tmpDir)
	if err != nil {
		t.Fatalf("SortScreenshots failed: %v", err)
	}

	// Verify photos in DB
	photos, err := db.ListPhotosByService(ctx, s1ID)
	if err != nil {
		t.Fatalf("failed to list photos for Abbrucharbeiten: %v", err)
	}

	// Expecting exactly 1 photo (img1.jpg)
	if len(photos) != 1 {
		t.Fatalf("expected 1 photo in DB under Abbrucharbeiten, got %d", len(photos))
	}

	p := photos[0]
	if p.FilePath != img1Path {
		t.Errorf("expected photo path %q, got %q", img1Path, p.FilePath)
	}
	if p.Source != "Client" {
		t.Errorf("expected source Client, got %q", p.Source)
	}
	if p.Status != "pending" {
		t.Errorf("expected status pending, got %q", p.Status)
	}
}
