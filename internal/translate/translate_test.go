package translate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranslate_Dictionary(t *testing.T) {
	// 1. Test word that exists in the dictionary (e.g. Badsanierung -> pl)
	res, err := Translate(context.Background(), "Badsanierung", "auto", "pl")
	if err != nil {
		t.Fatalf("Translate from dictionary failed: %v", err)
	}
	if res != "Remont łazienki" {
		t.Errorf("expected 'Remont łazienki' from dictionary, got %q", res)
	}

	// 2. Test another word from dictionary (e.g. Abbrucharbeiten -> en)
	resEn, err := Translate(context.Background(), "Abbrucharbeiten", "auto", "en")
	if err != nil {
		t.Fatalf("Translate to English failed: %v", err)
	}
	if resEn != "Demolition works" {
		t.Errorf("expected 'Demolition works', got %q", resEn)
	}
}

func TestTranslate_FallbackToAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q != "SomeUnknownWord" {
			t.Errorf("expected query 'SomeUnknownWord', got %q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[[["JakiesNieznaneSlowo","SomeUnknownWord",null,null,3]],null,"en"]`))
	}))
	defer server.Close()

	// Override API base URL for test
	defaultBaseURL := baseURL
	baseURL = server.URL
	defer func() { baseURL = defaultBaseURL }()

	res, err := Translate(context.Background(), "SomeUnknownWord", "en", "pl")
	if err != nil {
		t.Fatalf("Translate failed: %v", err)
	}

	if res != "JakiesNieznaneSlowo" {
		t.Errorf("expected 'JakiesNieznaneSlowo', got %q", res)
	}
}
