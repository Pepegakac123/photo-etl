package generator

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

type BananaClient struct {
	apiKey     string
	basePrompt string
	httpClient *http.Client
	baseURL    string // Can be overridden in tests
}

func NewBananaClient(apiKey, basePrompt string) *BananaClient {
	return &BananaClient{
		apiKey:     apiKey,
		basePrompt: basePrompt,
		httpClient: &http.Client{},
		baseURL:    "https://generativelanguage.googleapis.com/v1beta/models/imagen-3.0-generate-002:predict",
	}
}

type imagenPrediction struct {
	BytesBase64Encoded string `json:"bytesBase64Encoded"`
	MimeType           string `json:"mimeType"`
}

type imagenResponse struct {
	Predictions []imagenPrediction `json:"predictions"`
}

func (c *BananaClient) GenerateImage(ctx context.Context, serviceName, clientCountry, serviceDescription, outputPath string) error {
	// If no API key is set, output a mock development image
	if c.apiKey == "" {
		return c.writeMockImage(outputPath)
	}

	prompt := fmt.Sprintf(
		"Service name: %s. Client country: %s. Service description: %s.\n%s",
		serviceName, clientCountry, serviceDescription, c.basePrompt,
	)

	payload := map[string]interface{}{
		"instances": []map[string]interface{}{
			{
				"prompt": prompt,
			},
		},
		"parameters": map[string]interface{}{
			"sampleCount": 1,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal imagen request payload: %w", err)
	}

	apiURL := c.baseURL
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-goog-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to Gemini API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gemini api returned status code: %d", resp.StatusCode)
	}

	var imgResp imagenResponse
	if err := json.NewDecoder(resp.Body).Decode(&imgResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(imgResp.Predictions) == 0 {
		return fmt.Errorf("no predictions returned from Gemini API")
	}

	base64Data := imgResp.Predictions[0].BytesBase64Encoded
	imgBytes, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("failed to decode base64 image bytes: %w", err)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(outputPath, imgBytes, 0644); err != nil {
		return fmt.Errorf("failed to write generated image to %s: %w", outputPath, err)
	}

	return nil
}

func (c *BananaClient) writeMockImage(outputPath string) error {
	// A small valid 1x1 PNG pixel base64 encoded
	mockPngBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	data, err := base64.StdEncoding.DecodeString(mockPngBase64)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	return os.WriteFile(outputPath, data, 0644)
}
