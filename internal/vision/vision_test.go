package vision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyImage(t *testing.T) {
	// Create mock OpenAI API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key authorization, got %s", r.Header.Get("Authorization"))
		}

		// Decode request to verify structure
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type     string `json:"type"`
					Text     string `json:"text,omitempty"`
					ImageURL *struct {
						URL string `json:"url"`
					} `json:"image_url,omitempty"`
				} `json:"content"`
			} `json:"messages"`
			ResponseFormat *struct {
				Type string `json:"type"`
			} `json:"response_format"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		if payload.Model != "gpt-4o-mini" {
			t.Errorf("expected model gpt-4o-mini, got %s", payload.Model)
		}

		// Verify we got the image in data URI format
		hasImage := false
		for _, msg := range payload.Messages {
			for _, content := range msg.Content {
				if content.Type == "image_url" && content.ImageURL != nil {
					if len(content.ImageURL.URL) > 0 {
						hasImage = true
					}
				}
			}
		}
		if !hasImage {
			t.Errorf("expected request payload to contain an image URL")
		}

		// Respond with a mock response
		w.Header().Set("Content-Type", "application/json")
		response := `{
			"choices": [
				{
					"message": {
						"content": "{\"matches\": [{\"category\": \"Abbrucharbeiten\", \"confidence\": 0.95}]}"
					}
				}
			],
			"usage": {
				"prompt_tokens": 100,
				"completion_tokens": 10,
				"total_tokens": 110
			}
		}`
		w.Write([]byte(response))
	}))
	defer server.Close()

	// Create a temp image file
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.jpg")
	if err := os.WriteFile(imgPath, []byte("fake-image-bytes"), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	client := NewClient("test-key", "gpt-4o-mini", "Jesteś ekspertem...")
	client.baseURL = server.URL // override base URL for test

	categories := []string{"Abbrucharbeiten", "Fassadenbau"}
	results, promptTokens, completionTokens, err := client.ClassifyImage(context.Background(), imgPath, categories)
	if err != nil {
		t.Fatalf("ClassifyImage failed: %v", err)
	}

	if len(results) != 1 || results[0] != "Abbrucharbeiten" {
		t.Errorf("expected category 'Abbrucharbeiten', got %v", results)
	}

	if promptTokens != 100 || completionTokens != 10 {
		t.Errorf("expected promptTokens=100 and completionTokens=10, got %d and %d", promptTokens, completionTokens)
	}
}
