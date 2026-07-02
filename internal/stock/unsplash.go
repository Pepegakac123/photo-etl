package stock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type UnsplashClient struct {
	accessKey  string
	httpClient *http.Client
}

func NewUnsplashClient(accessKey string) *UnsplashClient {
	return &UnsplashClient{
		accessKey:  accessKey,
		httpClient: &http.Client{},
	}
}

func (c *UnsplashClient) AccessKey() string {
	return c.accessKey
}

func (c *UnsplashClient) TestConnection(ctx context.Context) error {
	if c.accessKey == "" {
		return fmt.Errorf("Unsplash access key is empty")
	}

	apiURL := "https://api.unsplash.com/search/photos?query=construction&page=1&per_page=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Client-ID "+c.accessKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("Unsplash API returned unauthorized (invalid access key)")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unsplash API returned status: %d", resp.StatusCode)
	}

	return nil
}

type unsplashSearchResponse struct {
	Results []struct {
		ID             string `json:"id"`
		Description    string `json:"description"`
		AltDescription string `json:"alt_description"`
		Urls           struct {
			Regular string `json:"regular"`
			Raw     string `json:"raw"`
		} `json:"urls"`
	} `json:"results"`
}

func (c *UnsplashClient) SearchPhotos(ctx context.Context, term string, page, pageSize int) ([]*EnvatoPhoto, error) {
	if c.accessKey == "" {
		return c.getMockPhotos(term, page, pageSize), nil
	}

	apiURL := fmt.Sprintf("https://api.unsplash.com/search/photos?query=%s&page=%d&per_page=%d",
		url.QueryEscape(term), page, pageSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create unsplash request: %w", err)
	}

	req.Header.Set("Authorization", "Client-ID "+c.accessKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call unsplash api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unsplash api returned unauthorized (invalid access key)")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unsplash api returned status code: %d", resp.StatusCode)
	}

	var res unsplashSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to parse unsplash response: %w", err)
	}

	var photos []*EnvatoPhoto
	for _, item := range res.Results {
		title := item.AltDescription
		if title == "" {
			title = item.Description
		}
		if title == "" {
			title = "Unsplash Photo"
		}

		photos = append(photos, &EnvatoPhoto{
			ID:         "unsplash-" + item.ID,
			Title:      title,
			PreviewURL: item.Urls.Regular,
			FullURL:    item.Urls.Raw,
		})
	}

	return photos, nil
}

func (c *UnsplashClient) getMockPhotos(term string, page, pageSize int) []*EnvatoPhoto {
	ec := NewEnvatoClient("")
	photos := ec.getMockPhotos(term, page, pageSize)
	var unsplashPhotos []*EnvatoPhoto
	for _, p := range photos {
		unsplashPhotos = append(unsplashPhotos, &EnvatoPhoto{
			ID:         "unsplash-mock-" + p.ID,
			Title:      p.Title,
			PreviewURL: p.PreviewURL,
			FullURL:    p.FullURL,
		})
	}
	return unsplashPhotos
}
