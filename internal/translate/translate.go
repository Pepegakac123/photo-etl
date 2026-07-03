package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

var baseURL = "https://translate.googleapis.com/translate_a/single"

// Translate translates a text using the free Google Translate single endpoint.
// It checks the static dictionary first, and falls back to the API.
func Translate(ctx context.Context, text, fromLang, toLang string) (string, error) {
	if text == "" {
		return "", nil
	}

	if val, ok := DictionaryTranslate(text, toLang); ok {
		return val, nil
	}

	apiURL := fmt.Sprintf("%s?client=gtx&sl=%s&tl=%s&dt=t&q=%s",
		baseURL, fromLang, toLang, url.QueryEscape(text))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create translation request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send translation request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("translation server returned status code: %d", resp.StatusCode)
	}

	var raw []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", fmt.Errorf("failed to decode translation response: %w", err)
	}

	if len(raw) == 0 || raw[0] == nil {
		return "", fmt.Errorf("invalid translation response format")
	}

	// Navigate raw[0] which is a slice of slices representing sentences
	sentences, ok := raw[0].([]interface{})
	if !ok {
		return "", fmt.Errorf("invalid structure for translation sentences")
	}

	var translated string
	for _, sentence := range sentences {
		sParts, ok := sentence.([]interface{})
		if !ok || len(sParts) == 0 {
			continue
		}
		txt, ok := sParts[0].(string)
		if ok {
			translated += txt
		}
	}

	return translated, nil
}
