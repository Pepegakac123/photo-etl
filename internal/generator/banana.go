package generator

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type BananaClient struct {
	apiKey     string
	basePrompt string
	httpClient *http.Client
	baseURL    string // Can be overridden in tests
	model      string
}

func NewBananaClient(apiKey, basePrompt string) *BananaClient {
	return &BananaClient{
		apiKey:     apiKey,
		basePrompt: basePrompt,
		httpClient: &http.Client{},
		baseURL:    "https://generativelanguage.googleapis.com/v1beta/interactions",
		model:      "gemini-3.1-flash-image",
	}
}

func (c *BananaClient) SetModel(model string) {
	c.model = model
}

func (c *BananaClient) Model() string {
	return c.model
}

func (c *BananaClient) TestConnection(ctx context.Context) error {
	if c.apiKey == "" {
		return fmt.Errorf("API key is empty")
	}

	payload := interactionsRequest{
		Model: c.model,
		Input: []contentPart{
			{
				Type: "text",
				Text: "ping",
			},
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("x-goog-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("interactions API returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responseFormatStruct struct {
	Type      string `json:"type"`
	ImageSize string `json:"image_size,omitempty"`
}

type interactionsRequest struct {
	Model          string                `json:"model"`
	Input          []contentPart         `json:"input"`
	ResponseFormat *responseFormatStruct `json:"response_format,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"`
}

type interactionStep struct {
	Type    string         `json:"type"`
	Content []contentBlock `json:"content"`
}

type interactionsResponse struct {
	Steps []interactionStep `json:"steps"`
}

func (c *BananaClient) GenerateImage(ctx context.Context, serviceName, clientCountry, serviceDescription, modelName, imageSize, outputPath string) error {
	// If no API key is set, output a mock development image
	if c.apiKey == "" {
		return c.writeMockImage(outputPath)
	}

	modelToUse := modelName
	if modelToUse == "" {
		modelToUse = c.model
	}
	if modelToUse == "" {
		modelToUse = "gemini-3.1-flash-image"
	}

	prompt := fmt.Sprintf(
		"Service name: %s. Client country: %s. Service description: %s.\n%s",
		serviceName, clientCountry, serviceDescription, c.basePrompt,
	)

	var responseFormat *responseFormatStruct
	if imageSize != "" {
		responseFormat = &responseFormatStruct{
			Type:      "image",
			ImageSize: imageSize,
		}
	} else {
		if modelToUse == "gemini-3.1-flash-image" || modelToUse == "gemini-3-pro-image" {
			responseFormat = &responseFormatStruct{
				Type:      "image",
				ImageSize: "2K",
			}
		} else if modelToUse == "gemini-3.1-flash-lite-image" {
			responseFormat = &responseFormatStruct{
				Type:      "image",
				ImageSize: "1K",
			}
		}
	}

	payload := interactionsRequest{
		Model:          modelToUse,
		Input: []contentPart{
			{
				Type: "text",
				Text: prompt,
			},
		},
		ResponseFormat: responseFormat,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal interactions request payload: %w", err)
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
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gemini api returned status code: %d, response: %s", resp.StatusCode, string(respBody))
	}

	var interactionsResp interactionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&interactionsResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	var base64Data string
	found := false
	// Search backwards through steps for the last generated image block
	for i := len(interactionsResp.Steps) - 1; i >= 0; i-- {
		step := interactionsResp.Steps[i]
		if step.Type == "model_output" {
			for j := len(step.Content) - 1; j >= 0; j-- {
				block := step.Content[j]
				if block.Type == "image" && block.Data != "" {
					base64Data = block.Data
					found = true
					break
				}
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("no image returned from Gemini API interactions endpoint")
	}

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

func (c *BananaClient) EditImage(ctx context.Context, inputImagePath, editPrompt, modelName, imageSize, outputPath string) error {
	if c.apiKey == "" {
		return c.writeMockImage(outputPath)
	}

	modelToUse := modelName
	if modelToUse == "" {
		modelToUse = "gemini-3.1-flash-image"
	}

	// Read original image bytes
	imgBytes, err := os.ReadFile(inputImagePath)
	if err != nil {
		return fmt.Errorf("failed to read input image: %w", err)
	}

	// Determine mime type
	mimeType := "image/png"
	if strings.HasSuffix(strings.ToLower(inputImagePath), ".jpg") || strings.HasSuffix(strings.ToLower(inputImagePath), ".jpeg") {
		mimeType = "image/jpeg"
	}

	base64Img := base64.StdEncoding.EncodeToString(imgBytes)

	type multiInputPart struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		Data     string `json:"data,omitempty"`
		MimeType string `json:"mime_type,omitempty"`
	}

	type editRequest struct {
		Model          string                `json:"model"`
		Input          []multiInputPart      `json:"input"`
		ResponseFormat *responseFormatStruct `json:"response_format,omitempty"`
	}

	var responseFormat *responseFormatStruct
	if imageSize != "" {
		responseFormat = &responseFormatStruct{
			Type:      "image",
			ImageSize: imageSize,
		}
	} else {
		responseFormat = &responseFormatStruct{
			Type:      "image",
			ImageSize: "1K", // Default to 1K for speed/cost
		}
	}

	payload := editRequest{
		Model: modelToUse,
		Input: []multiInputPart{
			{
				Type: "text",
				Text: editPrompt,
			},
			{
				Type:     "image",
				Data:     base64Img,
				MimeType: mimeType,
			},
		},
		ResponseFormat: responseFormat,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("x-goog-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to Gemini: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gemini returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var interactionsResp interactionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&interactionsResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	var outBase64 string
	found := false
	for i := len(interactionsResp.Steps) - 1; i >= 0; i-- {
		step := interactionsResp.Steps[i]
		if step.Type == "model_output" {
			for j := len(step.Content) - 1; j >= 0; j-- {
				block := step.Content[j]
				if block.Type == "image" && block.Data != "" {
					outBase64 = block.Data
					found = true
					break
				}
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("no image returned from Gemini edit request")
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(outBase64)
	if err != nil {
		return fmt.Errorf("failed to decode output image bytes: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	return os.WriteFile(outputPath, decodedBytes, 0644)
}

func (c *BananaClient) GenerateText(ctx context.Context, modelName, prompt string) (string, error) {
	if c.apiKey == "" {
		// Mock development response if no key is set
		return "Mocked enhanced text prompt description in English: professional view of repair work.", nil
	}

	modelToUse := modelName
	if modelToUse == "" {
		modelToUse = "gemini-2.5-flash-lite"
	}

	payload := interactionsRequest{
		Model: modelToUse,
		Input: []contentPart{
			{
				Type: "text",
				Text: prompt,
			},
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-goog-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini api returned status code: %d, response: %s", resp.StatusCode, string(respBody))
	}

	var interactionsResp interactionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&interactionsResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Search for text model output
	for i := len(interactionsResp.Steps) - 1; i >= 0; i-- {
		step := interactionsResp.Steps[i]
		if step.Type == "model_output" {
			for j := len(step.Content) - 1; j >= 0; j-- {
				block := step.Content[j]
				if block.Type == "text" && block.Text != "" {
					return block.Text, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no text output found in gemini response")
}
