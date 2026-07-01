package stock

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnvatoSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-envato-token" {
			t.Errorf("expected Bearer test-envato-token auth, got %q", r.Header.Get("Authorization"))
		}

		q := r.URL.Query()
		if q.Get("site") != "photodune.net" {
			t.Errorf("expected site=photodune.net, got %q", q.Get("site"))
		}
		if q.Get("term") != "bathroom renovation" {
			t.Errorf("expected term='bathroom renovation', got %q", q.Get("term"))
		}
		if q.Get("page") != "1" {
			t.Errorf("expected page=1, got %q", q.Get("page"))
		}

		w.Header().Set("Content-Type", "application/json")
		// Realistic mock Envato search JSON
		responseJSON := `{
			"total_hits": 1,
			"matches": [
				{
					"id": "11223344",
					"name": "Modern Bathroom Renovation",
					"previews": {
						"thumbnail_preview": {
							"large_url": "https://previews.envato.com/fake-preview.jpg"
						}
					}
				}
			]
		}`
		w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	client := NewEnvatoClient("test-envato-token")
	client.baseURL = server.URL // override base URL for test

	results, err := client.SearchPhotos(context.Background(), "bathroom renovation", 1, 10)
	if err != nil {
		t.Fatalf("SearchPhotos failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	item := results[0]
	if item.ID != "11223344" || item.Title != "Modern Bathroom Renovation" || item.PreviewURL != "https://previews.envato.com/fake-preview.jpg" {
		t.Errorf("unexpected item values: %+v", item)
	}
}
