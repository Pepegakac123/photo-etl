package generator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-gemini-key" {
			t.Errorf("expected x-goog-api-key = test-gemini-key, got %q", r.Header.Get("x-goog-api-key"))
		}

		var payload struct {
			Model string `json:"model"`
			Input []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"input"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if payload.Model != "gemini-3.1-flash-image" || len(payload.Input) != 1 || payload.Input[0].Text == "" {
			t.Errorf("invalid prompt payload: %+v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		// Mock response with interactions steps schema containing fake image bytes
		responseJSON := `{
			"steps": [
				{
					"type": "model_output",
					"content": [
						{
							"type": "image",
							"data": "ZmFrZS1pbWFnZS1ieXRlcw=="
						}
					]
				}
			]
		}`
		w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	client := NewBananaClient("test-gemini-key", "Zdjęcie musi wyglądać jak zrobione amatorsko...")
	client.baseURL = server.URL // override base URL for test

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "generated.png")

	err := client.GenerateImage(context.Background(), "Abbrucharbeiten", "Niemcy", "Rozbiórka starego budynku", "gemini-3.1-flash-image", "", outputPath, nil, nil)
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}

	// Verify file was written and has correct content
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	if string(data) != "fake-image-bytes" {
		t.Errorf("expected file content 'fake-image-bytes', got %q", string(data))
	}
}
