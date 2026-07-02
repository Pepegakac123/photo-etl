package stock

import (
	"context"
	"testing"
)

func TestUnsplashMockSearch(t *testing.T) {
	client := NewUnsplashClient("")
	ctx := context.Background()

	photos, err := client.SearchPhotos(ctx, "renovation", 1, 5)
	if err != nil {
		t.Fatalf("Expected no error from mock search, got: %v", err)
	}

	if len(photos) != 5 {
		t.Errorf("Expected 5 photos, got %d", len(photos))
	}

	for _, p := range photos {
		if p.ID == "" {
			t.Errorf("Expected non-empty photo ID")
		}
		if p.PreviewURL == "" || p.FullURL == "" {
			t.Errorf("Expected non-empty URLs")
		}
	}
}

func TestUnsplashTestConnectionEmpty(t *testing.T) {
	client := NewUnsplashClient("")
	ctx := context.Background()

	err := client.TestConnection(ctx)
	if err == nil {
		t.Errorf("Expected error from empty access key connection test")
	}
}
