package translate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranslate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q != "Badsanierung" {
			t.Errorf("expected query 'Badsanierung', got %q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		// Mock Google Translate single endpoint JSON structure
		w.Write([]byte(`[[["Remont łazienki","Badsanierung",null,null,3]],null,"de"]`))
	}))
	defer server.Close()

	// Override API base URL for test
	defaultBaseURL := baseURL
	baseURL = server.URL
	defer func() { baseURL = defaultBaseURL }()

	res, err := Translate(context.Background(), "Badsanierung", "auto", "pl")
	if err != nil {
		t.Fatalf("Translate failed: %v", err)
	}

	if res != "Remont łazienki" {
		t.Errorf("expected 'Remont łazienki', got %q", res)
	}
}
