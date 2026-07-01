package stock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type EnvatoPhoto struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	PreviewURL string `json:"preview_url"`
}

type EnvatoClient struct {
	token      string
	httpClient *http.Client
	baseURL    string // Can be overridden in tests
}

func NewEnvatoClient(token string) *EnvatoClient {
	return &EnvatoClient{
		token:      token,
		httpClient: &http.Client{},
		baseURL:    "https://api.envato.com/v1/discovery/search/search/item",
	}
}

func (c *EnvatoClient) Token() string {
	return c.token
}

func (c *EnvatoClient) TestConnection(ctx context.Context) error {
	if c.token == "" {
		return fmt.Errorf("API token is empty")
	}

	apiURL := fmt.Sprintf("%s?site=photodune.net&term=construction&page=1&page_size=1", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("Envato API returned unauthorized (invalid token)")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Envato API returned status: %d", resp.StatusCode)
	}

	return nil
}

type envatoSearchResponse struct {
	Items []struct {
		ID       interface{} `json:"id"` // can be float64 or string in JSON
		Name     string      `json:"name"`
		Previews struct {
			IconWithLandscapePreview struct {
				LandscapeURL string `json:"landscape_url"`
			} `json:"icon_with_landscape_preview"`
		} `json:"previews"`
	} `json:"items"`
}

func (c *EnvatoClient) SearchPhotos(ctx context.Context, term string, page, pageSize int) ([]*EnvatoPhoto, error) {
	// If no token is provided, return nice mock development photos
	if c.token == "" {
		return c.getMockPhotos(term, page, pageSize), nil
	}

	apiURL := fmt.Sprintf("%s?site=photodune.net&term=%s&page=%d&page_size=%d",
		c.baseURL, url.QueryEscape(term), page, pageSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create envato request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call envato api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("envato api returned unauthorized (invalid token)")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("envato api returned status code: %d", resp.StatusCode)
	}

	var res envatoSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to parse envato response: %w", err)
	}

	var photos []*EnvatoPhoto
	for _, item := range res.Items {
		// Convert ID to string
		var itemID string
		switch v := item.ID.(type) {
		case string:
			itemID = v
		case float64:
			itemID = strconv.FormatFloat(v, 'f', -1, 64)
		default:
			itemID = fmt.Sprintf("%v", v)
		}

		preview := item.Previews.IconWithLandscapePreview.LandscapeURL
		if preview == "" {
			continue // skip items without previews
		}

		photos = append(photos, &EnvatoPhoto{
			ID:         itemID,
			Title:      item.Name,
			PreviewURL: preview,
		})
	}

	return photos, nil
}

func (c *EnvatoClient) getMockPhotos(term string, page, pageSize int) []*EnvatoPhoto {
	// A collection of high-quality construction stock photo previews from Unsplash
	mockUrls := []string{
		"https://images.unsplash.com/photo-1541888946425-d81bb19240f5?w=500&auto=format&fit=crop&q=60", // Excavator
		"https://images.unsplash.com/photo-1504307651254-35680f356dfd?w=500&auto=format&fit=crop&q=60", // Worker
		"https://images.unsplash.com/photo-1590069261209-f8e9b8642343?w=500&auto=format&fit=crop&q=60", // Hard hat
		"https://images.unsplash.com/photo-1581094288338-2314dddb7ecc?w=500&auto=format&fit=crop&q=60", // Construction site
		"https://images.unsplash.com/photo-1486406146926-c627a92ad1ab?w=500&auto=format&fit=crop&q=60", // Modern building
		"https://images.unsplash.com/photo-1541535650810-10d26f5c2ab3?w=500&auto=format&fit=crop&q=60", // Tools
		"https://images.unsplash.com/photo-1534224039826-c7a0dea0e66a?w=500&auto=format&fit=crop&q=60", // Electrician
		"https://images.unsplash.com/photo-1621905251189-08b45d6a269e?w=500&auto=format&fit=crop&q=60", // Plumbing
		"https://images.unsplash.com/photo-1562259949-e8e7689d7828?w=500&auto=format&fit=crop&q=60", // Masonry
		"https://images.unsplash.com/photo-1503387762-592deb58ef4e?w=500&auto=format&fit=crop&q=60", // Architecture
	}

	var photos []*EnvatoPhoto
	startIndex := ((page - 1) * pageSize) % len(mockUrls)
	for i := 0; i < pageSize; i++ {
		idx := (startIndex + i) % len(mockUrls)
		id := fmt.Sprintf("mock-%d-%d", idx, page)
		photos = append(photos, &EnvatoPhoto{
			ID:         id,
			Title:      fmt.Sprintf("Mock %s - Photo %d", term, i+1),
			PreviewURL: mockUrls[idx],
		})
	}
	return photos
}
