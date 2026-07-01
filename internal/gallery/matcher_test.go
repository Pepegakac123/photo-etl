package gallery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Pepegakac123/photo-etl/internal/storage"
)

type mockTranslator struct {
	translations map[string]string
}

func (m *mockTranslator) Translate(ctx context.Context, text, fromLang, toLang string) (string, error) {
	if val, ok := m.translations[text]; ok {
		return val, nil
	}
	return text, nil
}

func TestGalleryService(t *testing.T) {
	// Create mock gallery directory structure
	tmpDir, err := os.MkdirTemp("", "gallery-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	folders := []string{
		"Badsanierung_Remont łazienki",
		"Fassadenbau_Elewacje",
		"Adaptacja Poddasza",
	}
	for _, folder := range folders {
		if err := os.Mkdir(filepath.Join(tmpDir, folder), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	// Initialize DB
	db, err := storage.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create service
	mt := &mockTranslator{
		translations: map[string]string{
			"Sanierung": "Remont",
		},
	}
	gs := NewService(db, mt, tmpDir)

	// Test Indexing
	err = gs.IndexGallery(ctx)
	if err != nil {
		t.Fatalf("IndexGallery failed: %v", err)
	}

	// Verify DB has indexed items
	indexed, err := db.ListGalleryFolders(ctx)
	if err != nil {
		t.Fatalf("failed to list gallery folders: %v", err)
	}
	if len(indexed) != 3 {
		t.Errorf("expected 3 indexed folders, got %d", len(indexed))
	}

	// Test Match 1: Direct Exact/High Match
	matches, isFallback, err := gs.MatchService(ctx, "Badsanierung")
	if err != nil {
		t.Fatalf("MatchService failed: %v", err)
	}
	if isFallback {
		t.Errorf("expected direct match, got fallback")
	}
	if len(matches) == 0 || matches[0].Folder.GermanName != "Badsanierung" {
		t.Errorf("expected Badsanierung as top match, got %+v", matches)
	}

	// Test Match 2: Translation Fallback Match
	// "Sanierung" doesn't have a direct matching folder with score >= 0.75,
	// so it translates to "Remont" which fuzzy-matches "Remont łazienki"
	matches, isFallback, err = gs.MatchService(ctx, "Sanierung")
	if err != nil {
		t.Fatalf("MatchService failed: %v", err)
	}
	if !isFallback {
		t.Errorf("expected fallback match for Sanierung, got direct")
	}
	if len(matches) == 0 || matches[0].Folder.GermanName != "Badsanierung" {
		t.Errorf("expected Badsanierung (Remont łazienki) as top match for Sanierung, got %+v", matches)
	}
}

func TestListPhotosInFolder(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create some mock images and a text file
	img1 := filepath.Join(tmpDir, "photo1.jpg")
	img2 := filepath.Join(tmpDir, "photo2.png")
	txt := filepath.Join(tmpDir, "notes.txt")
	
	_ = os.WriteFile(img1, []byte("fake"), 0644)
	_ = os.WriteFile(img2, []byte("fake"), 0644)
	_ = os.WriteFile(txt, []byte("fake"), 0644)
	
	gs := NewService(nil, nil, "")
	photos, err := gs.ListPhotosInFolder(tmpDir)
	if err != nil {
		t.Fatalf("ListPhotosInFolder failed: %v", err)
	}
	
	if len(photos) != 2 {
		t.Fatalf("expected 2 photos, got %d", len(photos))
	}
	
	hasImg1, hasImg2 := false, false
	for _, p := range photos {
		if filepath.Base(p) == "photo1.jpg" {
			hasImg1 = true
		} else if filepath.Base(p) == "photo2.png" {
			hasImg2 = true
		}
	}
	
	if !hasImg1 || !hasImg2 {
		t.Errorf("missing expected image file in list: %v", photos)
	}
}

