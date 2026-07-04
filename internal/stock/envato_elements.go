package stock

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// GetEnvatoElementsUnwatermarkedURL searches elements.envato.com/photos/ for a photo with the given title,
// navigates to its detail page (using injected cookies if provided), and extracts the unwatermarked preview image URL.
func GetEnvatoElementsUnwatermarkedURL(ctx context.Context, title string, rawCookies string) (string, error) {
	browserPath := "/usr/bin/chromium"

	l := launcher.New().Bin(browserPath).Headless(true).NoSandbox(true)
	controlURL, err := l.Launch()
	if err != nil {
		return "", fmt.Errorf("failed to launch browser: %w", err)
	}
	browser := rod.New().ControlURL(controlURL).MustConnect()
	defer browser.MustClose()

	page := stealth.MustPage(browser)

	// Set cookies if provided
	if rawCookies != "" {
		var cookies []*proto.NetworkCookieParam
		// Parse cookies from raw cookie string or key-value format
		lines := strings.Split(rawCookies, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			
			// Tab-separated format (clipboard copy from cookie manager)
			parts := strings.Split(line, "\t")
			if len(parts) >= 2 {
				name := parts[0]
				value := parts[1]
				domain := ".envato.com"
				if len(parts) >= 3 {
					domain = parts[2]
				}
				cookies = append(cookies, &proto.NetworkCookieParam{
					Name:   name,
					Value:  value,
					Domain: domain,
					Path:   "/",
				})
				continue
			}

			// Standard Cookie header format (name=value; name2=value2)
			semiParts := strings.Split(line, ";")
			for _, part := range semiParts {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				eqIdx := strings.Index(part, "=")
				if eqIdx != -1 {
					name := part[:eqIdx]
					value := part[eqIdx+1:]
					domain := ".envato.com"
					if strings.Contains(name, "elements.session") || strings.Contains(name, "_elements_session") {
						domain = ".elements.envato.com"
					}
					cookies = append(cookies, &proto.NetworkCookieParam{
						Name:   name,
						Value:  value,
						Domain: domain,
						Path:   "/",
					})
				}
			}
		}

		if len(cookies) > 0 {
			_ = page.SetCookies(cookies)
		}
	}

	// 1. Clean the title and construct a safe search query slug (at most 4 words to prevent Envato Elements routing errors)
	words := strings.Fields(strings.ToLower(title))
	limit := 4
	if len(words) < limit {
		limit = len(words)
	}
	cleanWords := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		w := regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(words[i], "")
		if w != "" {
			cleanWords = append(cleanWords, w)
		}
	}
	searchSlug := strings.Join(cleanWords, "-")
	if searchSlug == "" {
		searchSlug = "photos"
	}
	searchURL := fmt.Sprintf("https://elements.envato.com/photos/%s", searchSlug)

	if err := page.Navigate(searchURL); err != nil {
		return "", fmt.Errorf("failed to navigate to search page: %w", err)
	}
	page.MustWaitLoad()
	time.Sleep(3 * time.Second)

	dom := page.MustHTML()

	// Extract product detail page links matching -P[A-Z0-9]{7} and score them by title match
	reLink := regexp.MustCompile(`<a[^>]+href="([^"]+)"[^>]*>([\s\S]*?)</a>`)
	linkMatches := reLink.FindAllStringSubmatch(dom, -1)

	var detailLink string
	bestScore := -1
	seen := make(map[string]bool)

	for _, m := range linkMatches {
		href := m[1]
		content := m[2]

		if strings.Contains(href, "-P") && !strings.Contains(href, "similar-to") && !seen[href] {
			seen[href] = true

			// Extract alt text from the image inside the <a> tag
			altText := ""
			reAlt := regexp.MustCompile(`(?i)alt="([^"]+)"`)
			altMatch := reAlt.FindStringSubmatch(content)
			if len(altMatch) > 1 {
				altText = strings.TrimPrefix(altMatch[1], "Preview: ")
				altText = strings.TrimSpace(altText)
			}

			// Calculate a match score (higher is better)
			score := 0
			if altText != "" {
				if strings.EqualFold(altText, title) {
					score = 100 // Perfect match
				} else if strings.Contains(strings.ToLower(title), strings.ToLower(altText)) || strings.Contains(strings.ToLower(altText), strings.ToLower(title)) {
					score = 80
				} else {
					// Count common words
					originalWords := strings.Fields(strings.ToLower(title))
					resultWords := strings.Fields(strings.ToLower(altText))
					common := 0
					for _, ow := range originalWords {
						for _, rw := range resultWords {
							if ow == rw {
								common++
							}
						}
					}
					score = common
				}
			}

			if score > bestScore {
				bestScore = score
				detailLink = href
			}
		}
	}

	if detailLink == "" {
		return "", fmt.Errorf("no matching photo found on Envato Elements for title: %s", title)
	}

	if !strings.HasPrefix(detailLink, "http") {
		detailLink = "https://elements.envato.com" + detailLink
	}

	// 2. Navigate to product detail page
	if err := page.Navigate(detailLink); err != nil {
		return "", fmt.Errorf("failed to navigate to product page: %w", err)
	}
	page.MustWaitLoad()
	time.Sleep(3 * time.Second)

	detailDom := page.MustHTML()

	// Get page title (h1)
	reH1 := regexp.MustCompile(`(?i)<h1[^>]*>([^<]+)</h1>`)
	h1Match := reH1.FindStringSubmatch(detailDom)
	pageTitle := title
	if len(h1Match) > 1 {
		pageTitle = strings.TrimSpace(h1Match[1])
	}

	// Find the <img> tag whose alt matches the title
	reImg := regexp.MustCompile(`(?i)<img[^>]+alt="([^"]+)"[^>]*>`)
	imgMatches := reImg.FindAllStringSubmatch(detailDom, -1)

	var targetImgTag string
	for _, m := range imgMatches {
		altText := strings.TrimSpace(m[1])
		if strings.EqualFold(altText, pageTitle) || strings.EqualFold(altText, "Preview: "+pageTitle) {
			targetImgTag = m[0]
			break
		}
	}

	// If no exact alt match, fallback to the first image src containing elements-resized
	if targetImgTag == "" {
		reImgAll := regexp.MustCompile(`(?i)<img\b[^>]+>`)
		allImgs := reImgAll.FindAllString(detailDom, -1)
		for _, tag := range allImgs {
			if strings.Contains(tag, "elements-resized") && !strings.Contains(tag, "data-testid=\"img-photo-card\"") {
				targetImgTag = tag
				break
			}
		}
	}

	if targetImgTag == "" {
		return "", fmt.Errorf("could not find main photo image tag in details page DOM")
	}

	// 3. Extract the high-res URL from srcset or src
	reURL := regexp.MustCompile(`https://[a-zA-Z0-9.-]+\.envatousercontent\.com/[^"'\s,]+`)
	urls := reURL.FindAllString(targetImgTag, -1)

	if len(urls) == 0 {
		return "", fmt.Errorf("no image URLs found inside target image tag")
	}

	// Find the URL with the largest width parameter (e.g. w=1000 or w=1600)
	var bestURL string
	maxWidth := 0
	reW := regexp.MustCompile(`w=(\d+)`)
	for _, u := range urls {
		u = strings.ReplaceAll(u, "&amp;", "&")
		wMatch := reW.FindStringSubmatch(u)
		width := 500 // default fallback width
		if len(wMatch) > 1 {
			fmt.Sscanf(wMatch[1], "%d", &width)
		}
		if width > maxWidth {
			maxWidth = width
			bestURL = u
		}
	}

	if bestURL == "" {
		bestURL = strings.ReplaceAll(urls[0], "&amp;", "&")
	}

	return bestURL, nil
}
