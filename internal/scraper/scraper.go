package scraper

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FindBrowserPath searches for chromium or google-chrome on the system.
func FindBrowserPath() (string, error) {
	browsers := []string{"chromium", "google-chrome", "chrome", "chromium-browser"}
	for _, b := range browsers {
		if path, err := exec.LookPath(b); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("nie znaleziono przeglądarki Chromium na tym komputerze. Aby pobierać zdjęcia ze stron lub FB, zainstaluj Chromium (np. 'sudo pacman -S chromium' na Arch, lub 'sudo apt install chromium-browser' na Ubuntu/Debian)")
}

// ScrapePhotos scrapes all high-quality photos from Facebook or a general website.
// It downloads them to the specified output directory and reports progress via sendLog.
func ScrapePhotos(ctx context.Context, targetURL string, outputDir string, sendLog func(string)) error {
	if targetURL == "" {
		return fmt.Errorf("pusty adres URL")
	}

	sendLog(fmt.Sprintf("[SYSTEM] Inicjalizacja parsera dla URL: %s", targetURL))
	
	// 1. Check for Chromium
	browserPath, err := FindBrowserPath()
	if err != nil {
		return err
	}
	sendLog(fmt.Sprintf("[SYSTEM] Znaleziono przeglądarkę: %s", filepath.Base(browserPath)))

	// 2. Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// 3. Execute headless Chromium to render the page
	sendLog("[SYSTEM] Wczytywanie i renderowanie strony przez Chromium (headless)...")
	cmd := exec.CommandContext(ctx, browserPath, "--headless", "--disable-gpu", "--no-sandbox", "--dump-dom", targetURL)
	domBytes, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("błąd podczas renderowania strony: %w (upewnij się, czy Chromium działa prawidłowo)", err)
	}

	dom := string(domBytes)
	sendLog("[SYSTEM] Strona wczytana pomyślnie. Rozpoczynanie parsowania...")

	var imageUrls []string
	isFB := strings.Contains(targetURL, "facebook.com")

	if isFB {
		// Try to parse full-resolution images from JSON state ("viewer_image":{"uri":"..."})
		re := regexp.MustCompile(`"viewer_image"\s*:\s*\{\s*"uri"\s*:\s*"([^"]+)"`)
		matches := re.FindAllStringSubmatch(dom, -1)
		
		seen := make(map[string]bool)
		for _, m := range matches {
			uri := m[1]
			// Unescape JSON string
			uri = strings.ReplaceAll(uri, `\/`, "/")
			if !seen[uri] {
				seen[uri] = true
				imageUrls = append(imageUrls, uri)
			}
		}
		
		// Fallback: search for direct fbcdn image links in case viewer_image JSON is not present
		if len(imageUrls) == 0 {
			reFB := regexp.MustCompile(`https://scontent[^"'\s]*fbcdn.net/[^"'\s]*`)
			matchesFB := reFB.FindAllString(dom, -1)
			for _, uri := range matchesFB {
				uri = strings.ReplaceAll(uri, "&amp;", "&")
				// Keep unique
				if !seen[uri] {
					seen[uri] = true
					imageUrls = append(imageUrls, uri)
				}
			}
		}
		sendLog(fmt.Sprintf("[OK] Znaleziono %d potencjalnych zdjęć z Facebooka.", len(imageUrls)))
	} else {
		// General website scraping
		base, err := url.Parse(targetURL)
		if err != nil {
			return fmt.Errorf("niepoprawny format URL: %w", err)
		}

		// Find image sources
		seen := make(map[string]bool)
		
		// Match <img> src
		reImg := regexp.MustCompile(`(?i)<img\s+[^>]*src=["']([^"']+)["']`)
		matchesImg := reImg.FindAllStringSubmatch(dom, -1)
		for _, m := range matchesImg {
			ref := strings.TrimSpace(m[1])
			if ref != "" {
				u, err := base.Parse(ref)
				if err == nil {
					fullURL := u.String()
					if !seen[fullURL] {
						seen[fullURL] = true
						imageUrls = append(imageUrls, fullURL)
					}
				}
			}
		}

		// Match <img> data-src
		reDataSrc := regexp.MustCompile(`(?i)<img\s+[^>]*data-src=["']([^"']+)["']`)
		matchesDataSrc := reDataSrc.FindAllStringSubmatch(dom, -1)
		for _, m := range matchesDataSrc {
			ref := strings.TrimSpace(m[1])
			if ref != "" {
				u, err := base.Parse(ref)
				if err == nil {
					fullURL := u.String()
					if !seen[fullURL] {
						seen[fullURL] = true
						imageUrls = append(imageUrls, fullURL)
					}
				}
			}
		}

		// Match <a> href link to image
		reLink := regexp.MustCompile(`(?i)<a\s+[^>]*href=["']([^"']+\.(jpe?g|png|webp))["']`)
		matchesLink := reLink.FindAllStringSubmatch(dom, -1)
		for _, m := range matchesLink {
			ref := strings.TrimSpace(m[1])
			if ref != "" {
				u, err := base.Parse(ref)
				if err == nil {
					fullURL := u.String()
					if !seen[fullURL] {
						seen[fullURL] = true
						imageUrls = append(imageUrls, fullURL)
					}
				}
			}
		}
		sendLog(fmt.Sprintf("[OK] Znaleziono %d linków do obrazów na stronie.", len(imageUrls)))
	}

	if len(imageUrls) == 0 {
		return fmt.Errorf("nie znaleziono żadnych zdjęć na podanej stronie")
	}

	// 4. Download images with rate limiting/concurrency
	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	downloadCount := 0
	for i, imgURL := range imageUrls {
		// Skip tracker pixels, icons, and small social assets
		imgURLClean := strings.ToLower(imgURL)
		if !isFB {
			if strings.Contains(imgURLClean, "/logo") || strings.Contains(imgURLClean, "/icon") || 
			   strings.Contains(imgURLClean, "avatar") || strings.Contains(imgURLClean, "sprite") ||
			   strings.Contains(imgURLClean, "analytics") || strings.Contains(imgURLClean, "pixel") {
				continue
			}
		}

		sendLog(fmt.Sprintf("Pobieranie zdjęcia %d/%d...", i+1, len(imageUrls)))

		req, err := http.NewRequestWithContext(ctx, "GET", imgURL, nil)
		if err != nil {
			sendLog(fmt.Sprintf("  [ERR] Błąd zapytania dla zdjęcia %d: %v", i+1, err))
			continue
		}
		// Add browser headers
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")

		resp, err := client.Do(req)
		if err != nil {
			sendLog(fmt.Sprintf("  [ERR] Błąd pobierania zdjęcia %d: %v", i+1, err))
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			sendLog(fmt.Sprintf("  [ERR] Serwer zwrócił status %d dla zdjęcia %d", resp.StatusCode, i+1))
			continue
		}

		// Read and verify size (skip tiny images < 10KB)
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			sendLog(fmt.Sprintf("  [ERR] Błąd zapisu danych dla zdjęcia %d: %v", i+1, err))
			continue
		}

		if len(data) < 10240 { // 10 KB
			// Skip tiny assets
			continue
		}

		// Determine file extension
		ext := ".jpg"
		if strings.Contains(resp.Header.Get("Content-Type"), "png") {
			ext = ".png"
		} else if strings.Contains(resp.Header.Get("Content-Type"), "webp") {
			ext = ".webp"
		}

		// Create unique filename
		var filename string
		if isFB {
			filename = fmt.Sprintf("parsed_fb_%d_%d%s", time.Now().Unix(), downloadCount, ext)
		} else {
			filename = fmt.Sprintf("parsed_web_%d_%d%s", time.Now().Unix(), downloadCount, ext)
		}

		filePath := filepath.Join(outputDir, filename)
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			sendLog(fmt.Sprintf("  [ERR] Nie można zapisać pliku: %v", err))
			continue
		}

		downloadCount++
		log.Printf("Downloaded %s successfully to %s", imgURL, filePath)
	}

	sendLog(fmt.Sprintf("[OK] Pobieranie zakończone! Pomyślnie pobrano %d zdjęć do katalogu klienta.", downloadCount))
	return nil
}

// CleanFbDom cleans up temporary DOM dump files from disk if any exist.
func CleanFbDom(workdir string) {
	_ = os.Remove(filepath.Join(workdir, "fb_dom.html"))
	_ = os.Remove(filepath.Join(workdir, "fb_response.html"))
	_ = os.Remove(filepath.Join(workdir, "thumb.jpg"))
	_ = os.Remove(filepath.Join(workdir, "full.jpg"))
	_ = os.Remove(filepath.Join(workdir, "full_correct.jpg"))
}
