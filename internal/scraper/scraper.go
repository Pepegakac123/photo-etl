package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
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
func ScrapePhotos(ctx context.Context, targetURL string, outputDir string, fbCUser, fbXS string, sendLog func(string)) error {
	if targetURL == "" {
		return fmt.Errorf("pusty adres URL")
	}

	// Automatically rewrite Facebook URLs to target the photos tab if no tab is selected
	if strings.Contains(targetURL, "facebook.com") {
		if strings.Contains(targetURL, "profile.php") {
			if !strings.Contains(targetURL, "sk=") {
				u, err := url.Parse(targetURL)
				if err == nil {
					q := u.Query()
					q.Set("sk", "photos")
					u.RawQuery = q.Encode()
					targetURL = u.String()
				}
			}
		}
	}

	sendLog(fmt.Sprintf("[SYSTEM] Inicjalizacja parsera dla URL: %s", targetURL))
	
	// 1. Check for Chromium
	browserPath, err := FindBrowserPath()
	if err != nil {
		return err
	}
	sendLog(fmt.Sprintf("[SYSTEM] Uruchamianie Chromium (%s) w trybie headless i stealth...", filepath.Base(browserPath)))

	// 2. Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// 3. Launch browser using launcher and stealth
	l := launcher.New().Bin(browserPath).Headless(true).NoSandbox(true)
	controlURL, err := l.Launch()
	if err != nil {
		return fmt.Errorf("nie udało się uruchomić przeglądarki: %w", err)
	}
	
	browser := rod.New().ControlURL(controlURL).MustConnect()
	defer browser.MustClose()

	sendLog("[SYSTEM] Wczytywanie i renderowanie strony...")
	page := stealth.MustPage(browser)
	
	// Inject Facebook login cookies if provided
	if fbCUser != "" && fbXS != "" && strings.Contains(targetURL, "facebook.com") {
		cUserClean, err := url.QueryUnescape(fbCUser)
		if err != nil {
			cUserClean = fbCUser
		}
		xsClean, err := url.QueryUnescape(fbXS)
		if err != nil {
			xsClean = fbXS
		}

		sendLog("[SYSTEM] Logowanie do Facebooka przy użyciu podanych ciasteczek sesji...")
		err = page.SetCookies([]*proto.NetworkCookieParam{
			{
				Name:     "c_user",
				Value:    cUserClean,
				Domain:   ".facebook.com",
				Path:     "/",
				HTTPOnly: true,
				Secure:   true,
			},
			{
				Name:     "xs",
				Value:    xsClean,
				Domain:   ".facebook.com",
				Path:     "/",
				HTTPOnly: true,
				Secure:   true,
			},
		})
		if err != nil {
			sendLog(fmt.Sprintf("  [WARN] Błąd ustawiania ciasteczek logowania: %v", err))
		} else {
			sendLog("  [OK] Ciasteczka sesji zaimportowane pomyślnie.")
		}
	}
	
	if err := page.Navigate(targetURL); err != nil {
		return fmt.Errorf("błąd wczytywania strony: %w", err)
	}
	page.MustWaitLoad()

	// Wait 3 seconds for dynamic overlays (cookie banners, modals) to appear
	time.Sleep(3 * time.Second)

	// Helper to bypass Facebook cookie banner and login modal
	bypassOverlays := func() {
		_, _ = page.Eval(`() => {
			// 1. Remove cookie consent banner if present
			const cookieSpan = Array.from(document.querySelectorAll('span')).find(el => el.textContent.includes("Zezwól na wszystkie pliki cookie") || el.textContent.includes("Allow all"));
			if (cookieSpan) {
				let parent = cookieSpan.parentElement;
				while (parent && parent !== document.body) {
					const role = parent.getAttribute('role');
					const style = window.getComputedStyle(parent);
					if (role === 'dialog' || parent.tagName === 'FORM' || style.position === 'fixed') {
						parent.remove();
						break;
					}
					parent = parent.parentElement;
				}
			}

			// 2. Remove login modal (DO NOT click close button to avoid redirect, just delete from DOM)
			const loginSpan = Array.from(document.querySelectorAll('span')).find(el => el.textContent.includes("Wyświetl więcej na Facebooku") || el.textContent.includes("See more on Facebook"));
			if (loginSpan) {
				let parent = loginSpan.parentElement;
				while (parent && parent !== document.body) {
					const role = parent.getAttribute('role');
					const style = window.getComputedStyle(parent);
					if (role === 'dialog' || style.position === 'fixed') {
						parent.remove();
						break;
					}
					parent = parent.parentElement;
				}
			}

			// 3. Restore scrollability
			document.body.style.setProperty("overflow", "auto", "important");
			document.documentElement.style.setProperty("overflow", "auto", "important");
			document.body.style.setProperty("position", "relative", "important");
		}`)
	}

	// First bypass
	bypassOverlays()
	time.Sleep(1 * time.Second)

	isFB := strings.Contains(targetURL, "facebook.com")
	needScroll := true
	if isFB && strings.Contains(targetURL, "/media/set/") {
		needScroll = false
	}

	if needScroll {
		sendLog("[SYSTEM] Przewijanie strony w celu załadowania siatki zdjęć...")
		// Scroll down 15 times to trigger lazy-loading of photo grid items
		for i := 0; i < 15; i++ {
			sendLog(fmt.Sprintf("  Przewijanie strony (%d/15)...", i+1))
			bypassOverlays()
			
			// Scroll window and any scrollable containers
			_, _ = page.Eval(`() => {
				const amt = 400;
				window.scrollBy(0, amt);
				document.querySelectorAll('div').forEach(el => {
					if (el.scrollHeight > el.clientHeight) {
						const style = window.getComputedStyle(el);
						if (style.overflowY === 'auto' || style.overflowY === 'scroll') {
							el.scrollTop += amt;
						}
					}
				});
			}`)
			time.Sleep(1500 * time.Millisecond)
		}
	} else {
		sendLog("[SYSTEM] Profil Facebook: Pomijanie wstępnego scrollowania (bezpośredni link do albumu).")
	}

	sendLog("[SYSTEM] Parsowanie struktury strony i ekstrakcja zdjęć...")
	dom := page.MustHTML()

	var imageUrls []string

	if isFB {
		seen := make(map[string]bool)
		
		extractImagesFromDom := func(htmlContent string) {
			// Extract viewer_image JSON
			reViewer := regexp.MustCompile(`"viewer_image"\s*:\s*\{\s*"uri"\s*:\s*"([^"]+)"`)
			matchesViewer := reViewer.FindAllStringSubmatch(htmlContent, -1)
			for _, m := range matchesViewer {
				uri := m[1]
				uri = strings.ReplaceAll(uri, `\/`, "/")
				if !seen[uri] {
					seen[uri] = true
					imageUrls = append(imageUrls, uri)
				}
			}
			
			// Extract direct fbcdn links
			reFB := regexp.MustCompile(`https://scontent[^"'\s]*fbcdn.net/[^"'\s]*`)
			matchesFB := reFB.FindAllString(htmlContent, -1)
			for _, uri := range matchesFB {
				uri = strings.ReplaceAll(uri, "&amp;", "&")
				if !seen[uri] {
					seen[uri] = true
					imageUrls = append(imageUrls, uri)
				}
			}
		}

		// 1. Extract from the initial page
		extractImagesFromDom(dom)

		// 2. Automatically find and crawl album links if we are on a general photos/profile page
		if strings.Contains(targetURL, "sk=photos") || strings.Contains(targetURL, "/profile.php") {
			// Find any direct /media/set/?set=a.ID links in the DOM first
			reAlbumDirect := regexp.MustCompile(`/media/set/\?set=a\.(\d+)`)
			directMatches := reAlbumDirect.FindAllStringSubmatch(dom, -1)
			
			albumIDs := make(map[string]bool)
			for _, m := range directMatches {
				albumIDs[m[1]] = true
			}

			// Also extract any set=a.ID directly present in links on the page
			reSetDirect := regexp.MustCompile(`set=a\.(\d+)`)
			setMatchesDirect := reSetDirect.FindAllStringSubmatch(dom, -1)
			for _, m := range setMatchesDirect {
				albumIDs[m[1]] = true
			}

			// Extract photo viewer links to discover timeline/system albums
			reA := regexp.MustCompile(`<a[^>]+href="([^"]+)"`)
			matchesA := reA.FindAllStringSubmatch(dom, -1)
			
			var photoURLs []string
			seenPhotos := make(map[string]bool)
			for _, m := range matchesA {
				href := m[1]
				href = strings.ReplaceAll(href, "&amp;", "&")
				if (strings.Contains(href, "photo.php") || strings.Contains(href, "/photo/")) && !strings.Contains(href, "/photo_albums") {
					var absoluteURL string
					if strings.HasPrefix(href, "http") {
						absoluteURL = href
					} else {
						absoluteURL = "https://www.facebook.com" + href
					}
					if !seenPhotos[absoluteURL] {
						seenPhotos[absoluteURL] = true
						photoURLs = append(photoURLs, absoluteURL)
					}
				}
			}

			if len(photoURLs) > 0 {
				sendLog(fmt.Sprintf("[SYSTEM] Analiza %d wykrytych linków zdjęć w celu identyfikacji albumów...", len(photoURLs)))
				
				// Sample 25 photos: first 5 (recent timeline) + 20 distributed remaining (older albums)
				var sampledURLs []string
				for i := 0; i < 5 && i < len(photoURLs); i++ {
					sampledURLs = append(sampledURLs, photoURLs[i])
				}
				if len(photoURLs) > 5 {
					remaining := photoURLs[5:]
					sampleSize := 20
					if len(remaining) <= sampleSize {
						sampledURLs = append(sampledURLs, remaining...)
					} else {
						step := len(remaining) / sampleSize
						for i := 0; i < sampleSize; i++ {
							sampledURLs = append(sampledURLs, remaining[i*step])
						}
					}
				}

				cUserClean, _ := url.QueryUnescape(fbCUser)
				xsClean, _ := url.QueryUnescape(fbXS)

				clientForAlbums := &http.Client{
					Timeout: 5 * time.Second,
				}
				reSetQuery := regexp.MustCompile(`set=a\.(\d+)`)

				for idx, pURL := range sampledURLs {
					// Early exit: if we have found Cover, Profile, Timeline and Mobile Uploads (4 albums), we are done!
					if len(albumIDs) >= 4 {
						break
					}

					// Optimization: if it already has set=a.ID, extract it directly without fetching
					if reSetQuery.MatchString(pURL) {
						m := reSetQuery.FindStringSubmatch(pURL)
						albumID := m[1]
						if !albumIDs[albumID] {
							albumIDs[albumID] = true
							sendLog(fmt.Sprintf("  [OK] Wykryto ID albumu z adresu URL: %s (Link: https://www.facebook.com/media/set/?set=a.%s&type=3)", albumID, albumID))
						}
						continue
					}

					// Otherwise, fetch the photo page via HTTP GET with cookies (lightning fast!)
					u, err := url.Parse(pURL)
					if err == nil {
						fbid := u.Query().Get("fbid")
						sendLog(fmt.Sprintf("  Sprawdzanie metadanych zdjęcia %d/%d (fbid=%s)...", idx+1, len(sampledURLs), fbid))
						
						req, err := http.NewRequestWithContext(ctx, "GET", pURL, nil)
						if err == nil {
							req.Header.Set("Cookie", fmt.Sprintf("c_user=%s; xs=%s", cUserClean, xsClean))
							req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
							req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
							
							resp, err := clientForAlbums.Do(req)
							if err == nil {
								bodyBytes, err := io.ReadAll(resp.Body)
								resp.Body.Close()
								if err == nil {
									body := string(bodyBytes)
									setMatches := reSetQuery.FindAllStringSubmatch(body, -1)
									for _, sm := range setMatches {
										albumID := sm[1]
										if !albumIDs[albumID] {
											albumIDs[albumID] = true
											sendLog(fmt.Sprintf("  [OK] Wykryto ID albumu z metadanych zdjęcia: %s (Link: https://www.facebook.com/media/set/?set=a.%s&type=3)", albumID, albumID))
										}
									}
								}
							}
						}
					}
				}
			}

			// Convert discovered album IDs to URLs
			var albumURLs []string
			for id := range albumIDs {
				albumURLs = append(albumURLs, fmt.Sprintf("https://www.facebook.com/media/set/?set=a.%s&type=3&locale=pl_PL", id))
			}
			
			if len(albumURLs) > 0 {
				sendLog(fmt.Sprintf("[SYSTEM] Rozpoczynanie skanowania %d zidentyfikowanych albumów...", len(albumURLs)))
				for idx, albumURL := range albumURLs {
					reID := regexp.MustCompile(`set=a\.(\d+)`)
					albumID := "nieznany"
					if m := reID.FindStringSubmatch(albumURL); len(m) > 1 {
						albumID = m[1]
					}
					
					sendLog(fmt.Sprintf("  Skanowanie albumu %d/%d (ID: %s)...", idx+1, len(albumURLs), albumID))
					
					if err := page.Navigate(albumURL); err == nil {
						page.MustWaitLoad()
						time.Sleep(3 * time.Second)
						bypassOverlays()
						
						// Scroll 25 times per album to dynamically load all photos
						for s := 0; s < 25; s++ {
							bypassOverlays()
							_, _ = page.Eval(`() => {
								const amt = 400;
								window.scrollBy(0, amt);
								document.querySelectorAll('div').forEach(el => {
									if (el.scrollHeight > el.clientHeight) {
										const style = window.getComputedStyle(el);
										if (style.overflowY === 'auto' || style.overflowY === 'scroll') {
											el.scrollTop += amt;
										}
									}
								});
							}`)
							time.Sleep(1200 * time.Millisecond)
						}
						
						albumDom := page.MustHTML()
						beforeCount := len(imageUrls)
						extractImagesFromDom(albumDom)
						addedCount := len(imageUrls) - beforeCount
						sendLog(fmt.Sprintf("  [OK] Skanowanie albumu %s zakończone (znaleziono %d zdjęć).", albumID, addedCount))
					}
				}
			}
		}

		sendLog(fmt.Sprintf("[OK] Znaleziono łącznie %d zdjęć na profilu Facebook i w albumach.", len(imageUrls)))
	} else {
		// General website scraping
		base, err := url.Parse(targetURL)
		if err != nil {
			return fmt.Errorf("niepoprawny format URL: %w", err)
		}

		// Find image sources
		seen := make(map[string]bool)

		cleanWixURL := func(rawURL string) string {
			if strings.Contains(rawURL, "static.wixstatic.com/media/") {
				if idx := strings.Index(rawURL, "/v1/"); idx != -1 {
					rawURL = rawURL[:idx]
				}
				if idx := strings.Index(rawURL, "?"); idx != -1 {
					rawURL = rawURL[:idx]
				}
				if idx := strings.Index(rawURL, "#"); idx != -1 {
					rawURL = rawURL[:idx]
				}
			}
			return rawURL
		}

		// 1. Direct Regex Search for Wixstatic Media (highly robust for Wix sites like Wix galleries)
		reWix := regexp.MustCompile(`https?://static\.wixstatic\.com/media/[^"'\s>\)]+`)
		wixMatches := reWix.FindAllString(dom, -1)
		for _, m := range wixMatches {
			cleanURL := cleanWixURL(m)
			if !seen[cleanURL] {
				seen[cleanURL] = true
				imageUrls = append(imageUrls, cleanURL)
			}
		}

		// 2. Match <img> src
		reImg := regexp.MustCompile(`(?i)<img\s+[^>]*src=["']([^"']+)["']`)
		matchesImg := reImg.FindAllStringSubmatch(dom, -1)
		for _, m := range matchesImg {
			ref := strings.TrimSpace(m[1])
			if ref != "" {
				u, err := base.Parse(ref)
				if err == nil {
					fullURL := cleanWixURL(u.String())
					if !seen[fullURL] {
						seen[fullURL] = true
						imageUrls = append(imageUrls, fullURL)
					}
				}
			}
		}

		// 3. Match <img> data-src
		reDataSrc := regexp.MustCompile(`(?i)<img\s+[^>]*data-src=["']([^"']+)["']`)
		matchesDataSrc := reDataSrc.FindAllStringSubmatch(dom, -1)
		for _, m := range matchesDataSrc {
			ref := strings.TrimSpace(m[1])
			if ref != "" {
				u, err := base.Parse(ref)
				if err == nil {
					fullURL := cleanWixURL(u.String())
					if !seen[fullURL] {
						seen[fullURL] = true
						imageUrls = append(imageUrls, fullURL)
					}
				}
			}
		}

		// 4. Match <a> href link to image
		reLink := regexp.MustCompile(`(?i)<a\s+[^>]*href=["']([^"']+\.(jpe?g|png|webp))["']`)
		matchesLink := reLink.FindAllStringSubmatch(dom, -1)
		for _, m := range matchesLink {
			ref := strings.TrimSpace(m[1])
			if ref != "" {
				u, err := base.Parse(ref)
				if err == nil {
					fullURL := cleanWixURL(u.String())
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

	// 4. Download images
	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	downloadCount := 0
	skippedCount := 0
	for i, imgURL := range imageUrls {
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

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			sendLog(fmt.Sprintf("  [ERR] Błąd zapisu danych dla zdjęcia %d: %v", i+1, err))
			continue
		}

		if len(data) < 10240 { // skip tiny images < 10KB
			skippedCount++
			continue
		}

		ext := ".jpg"
		if strings.Contains(resp.Header.Get("Content-Type"), "png") {
			ext = ".png"
		} else if strings.Contains(resp.Header.Get("Content-Type"), "webp") {
			ext = ".webp"
		}

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
	}

	sendLog(fmt.Sprintf("[OK] Pobieranie zakończone! Pomyślnie pobrano %d wysokiej jakości zdjęć do katalogu klienta (pominięto %d małych miniatur/ikon).", downloadCount, skippedCount))
	return nil
}

// CleanFbDom cleans up temporary DOM dump files from disk if any exist.
func CleanFbDom(workdir string) {
	_ = os.Remove(filepath.Join(workdir, "fb_dom.html"))
	_ = os.Remove(filepath.Join(workdir, "fb_response.html"))
	_ = os.Remove(filepath.Join(workdir, "fb_initial.png"))
	_ = os.Remove(filepath.Join(workdir, "fb_after_cookie.png"))
	_ = os.Remove(filepath.Join(workdir, "fb_scrolled.png"))
	_ = os.Remove(filepath.Join(workdir, "fb_album_after_cookie.png"))
	_ = os.Remove(filepath.Join(workdir, "fb_album_scrolled.png"))
	_ = os.Remove(filepath.Join(workdir, "fb_elements.txt"))
	_ = os.Remove(filepath.Join(workdir, "mbasic_fb.html"))
}
