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
			Instances []struct {
				Prompt string `json:"prompt"`
			} `json:"instances"`
			Parameters struct {
				SampleCount int `json:"sampleCount"`
			} `json:"parameters"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if len(payload.Instances) != 1 || payload.Instances[0].Prompt == "" {
			t.Errorf("invalid prompt payload")
		}

		w.Header().Set("Content-Type", "application/json")
		// Mock response with a small base64 string for "fake" image bytes
		responseJSON := `{
			"predictions": [
				{
					"bytesBase64Encoded": "ZmFrZS1pbWFnZS1ieXRlcw==",
					"mimeType": "image/png"
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

	err := client.GenerateImage(context.Background(), "Abbrucharbeiten", "Niemcy", "Rozbiórka starego budynku", outputPath)
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
