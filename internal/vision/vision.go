package vision

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

type Client struct {
	apiKey     string
	model      string
	prompt     string
	httpClient *http.Client
	baseURL    string // Can be overridden in tests
}

func NewClient(apiKey, model, prompt string) *Client {
	return &Client{
		apiKey:     apiKey,
		model:      model,
		prompt:     prompt,
		httpClient: &http.Client{},
		baseURL:    "https://api.openai.com/v1/chat/completions",
	}
}

type chatMessageContent struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	ImageURL *chatMessageImage `json:"image_url,omitempty"`
}

type chatMessageImage struct {
	URL string `json:"url"`
}

type chatMessage struct {
	Role    string               `json:"role"`
	Content []chatMessageContent `json:"content"`
}

type chatCompletionRequest struct {
	Model          string              `json:"model"`
	Messages       []chatMessage       `json:"messages"`
	ResponseFormat *chatResponseFormat `json:"response_format,omitempty"`
}

type chatResponseFormat struct {
	Type string `json:"type"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *Client) Model() string {
	return c.model
}

func (c *Client) TestConnection(ctx context.Context) error {
	if c.apiKey == "" {
		return fmt.Errorf("API key is empty")
	}

	reqPayload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{
				Role: "user",
				Content: []chatMessageContent{
					{
						Type: "text",
						Text: "Say OK",
					},
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI API returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func mimeTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// ClassifyImage sends an image to the OpenAI Vision API and returns matching category names with >= 90% confidence.
func (c *Client) ClassifyImage(ctx context.Context, imagePath string, categories []string) ([]string, int, int, error) {
	// Read image and encode to base64
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to read image file: %w", err)
	}
	base64Image := base64.StdEncoding.EncodeToString(data)
	mimeType := mimeTypeFromPath(imagePath)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Image)

	// Build the detailed prompt instructing the model about the categories
	systemInstructions := fmt.Sprintf(
		"%s\nAvailable categories: %s.\nAnalyze the image and return all categories that match the image content with confidence scores (from 0.0 to 1.0). You MUST return a JSON object in this exact format: {\"matches\": [{\"category\": \"<matched category>\", \"confidence\": <score between 0.0 and 1.0>}]}. If the image represents trash, spam, or matches nothing, return an empty matches array.",
		c.prompt,
		strings.Join(categories, ", "),
	)

	reqPayload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{
				Role: "system",
				Content: []chatMessageContent{
					{
						Type: "text",
						Text: systemInstructions,
					},
				},
			},
			{
				Role: "user",
				Content: []chatMessageContent{
					{
						Type: "text",
						Text: "Classify this image into all matching categories with confidence scores.",
					},
					{
						Type: "image_url",
						ImageURL: &chatMessageImage{
							URL: dataURL,
						},
					},
				},
			},
		},
		ResponseFormat: &chatResponseFormat{
			Type: "json_object",
		},
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to send request to OpenAI API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, 0, fmt.Errorf("unexpected status code from OpenAI API: %d", resp.StatusCode)
	}

	var respPayload chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&respPayload); err != nil {
		return nil, 0, 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(respPayload.Choices) == 0 {
		return nil, 0, 0, fmt.Errorf("no choices returned from OpenAI API")
	}

	rawContent := respPayload.Choices[0].Message.Content

	type categoryMatch struct {
		Category   string  `json:"category"`
		Confidence float64 `json:"confidence"`
	}
	var classification struct {
		Matches []categoryMatch `json:"matches"`
	}
	if err := json.Unmarshal([]byte(rawContent), &classification); err != nil {
		return nil, respPayload.Usage.PromptTokens, respPayload.Usage.CompletionTokens, fmt.Errorf("failed to parse classification result JSON (%s): %w", rawContent, err)
	}

	var matchedCategories []string
	for _, m := range classification.Matches {
		if m.Confidence >= 0.90 {
			categoryClean := strings.TrimSpace(m.Category)
			for _, cat := range categories {
				if strings.EqualFold(cat, categoryClean) {
					matchedCategories = append(matchedCategories, cat)
					break
				}
			}
		}
	}

	return matchedCategories, respPayload.Usage.PromptTokens, respPayload.Usage.CompletionTokens, nil
}
