package ui

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Pepegakac123/photo-etl/internal/ingest"
	"github.com/Pepegakac123/photo-etl/internal/storage"
	"github.com/Pepegakac123/photo-etl/internal/vision"
)

func (s *Server) handleClientChange(w http.ResponseWriter, r *http.Request) {
	err := s.tmpl.ExecuteTemplate(w, "client_form", nil)
	if err != nil {
		log.Printf("Template client_form rendering error: %v", err)
	}
}

func (s *Server) handleClientSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientPath := r.FormValue("client_path")
	if clientPath == "" {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("Ścieżka katalogu jest wymagana"))
		return
	}

	absPath, err := filepath.Abs(clientPath)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf("Niepoprawna ścieżka: %v", err)))
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf("Katalog nie istnieje: %v", err)))
		return
	}
	if !info.IsDir() {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("Podana ścieżka nie jest katalogiem"))
		return
	}

	dbPath := filepath.Join(absPath, "photo_etl.db")
	db, err := storage.InitDB(dbPath)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf("Błąd bazy danych: %v", err)))
		return
	}

	s.db = db
	s.clientDir = absPath
	s.galleryService.SetDB(db)

	ctx := r.Context()
	services, err := db.ListServices(ctx)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf("Błąd wczytywania usług: %v", err)))
		return
	}

	if len(services) == 0 {
		scanner := ingest.NewScanner(db)
		res, err := scanner.Scan(ctx, absPath)
		if err != nil {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(fmt.Sprintf("Błąd skanowania katalogu: %v", err)))
			return
		}



		log.Printf("Dynamically wczytano klienta. Zarejestrowano %d usług.", len(res.ServicesAdded))
	} else {
		log.Printf("Dynamically wznowiono klienta. Znaleziono %d usług w DB.", len(services))
	}

	// Index gallery in background
	if s.cfg.LocalGalleryPath != "" {
		if _, err := os.Stat(s.cfg.LocalGalleryPath); err == nil {
			go func() {
				_ = s.galleryService.IndexGallery(context.Background())
			}()
		}
	}

	w.Header().Set("HX-Refresh", "true")
}

func (s *Server) handleSortScreenshotsStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	send := func(event, data string) {
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	sendLog := func(msg string) {
		send("log", msg)
	}

	sendProgress := func(pct int, status string) {
		send("progress", strconv.Itoa(pct))
		send("status", status)
	}

	sendLog("[SYSTEM] Rozpoczynanie klasyfikacji AI Vision...")

	if s.clientDir == "" || s.db == nil {
		sendLog("[ERR] Najpierw wczytaj folder klienta.")
		return
	}

	if s.cfg.OpenAIApiKey == "" {
		sendLog("[ERR] Brak skonfigurowanego klucza OpenAI w Ustawieniach. Przerwanie.")
		return
	}

	// Detect screenshots folder
	var screenshotsFolder string
	entries, err := os.ReadDir(s.clientDir)
	if err != nil {
		sendLog(fmt.Sprintf("[ERR] Błąd odczytu katalogu klienta: %v", err))
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			nameLower := strings.ToLower(entry.Name())
			if strings.Contains(nameLower, "whatsapp") || strings.Contains(nameLower, "zrzuty") {
				screenshotsFolder = filepath.Join(s.clientDir, entry.Name())
				break
			}
		}
	}

	if screenshotsFolder == "" {
		sendLog("[ERR] Nie znaleziono folderu WhatsApp ani zrzutów ekranu w katalogu klienta.")
		return
	}

	sendLog(fmt.Sprintf("[SYSTEM] Wykryto folder zrzutów: %s", screenshotsFolder))

	files, err := os.ReadDir(screenshotsFolder)
	if err != nil {
		sendLog(fmt.Sprintf("[ERR] Błąd odczytu folderu zrzutów: %v", err))
		return
	}

	processedPaths, err := s.db.GetAllPhotoPaths(ctx)
	if err != nil {
		sendLog(fmt.Sprintf("[ERR] Błąd odczytu bazy danych: %v", err))
		return
	}

	// Filter only image files that are not already in DB
	var imageFiles []os.DirEntry
	for _, file := range files {
		if !file.IsDir() && scanIsImage(file.Name()) {
			filePath := filepath.Join(screenshotsFolder, file.Name())
			if !processedPaths[filePath] {
				imageFiles = append(imageFiles, file)
			}
		}
	}

	if len(imageFiles) == 0 {
		sendLog("[SYSTEM] Brak nowych (nieposortowanych) zdjęć w folderze zrzutów do sklasyfikowania.")
		sendProgress(100, "Zakończono!")
		send("complete", "done")
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode == "test" {
		if len(imageFiles) > 5 {
			imageFiles = imageFiles[:5]
			sendLog(fmt.Sprintf("[SYSTEM] Tryb testowy: Ograniczono klasyfikację do pierwszych 5 nieposortowanych zdjęć."))
		} else {
			sendLog(fmt.Sprintf("[SYSTEM] Tryb testowy: Znaleziono %d nieposortowanych zdjęć.", len(imageFiles)))
		}
	} else {
		sendLog(fmt.Sprintf("[SYSTEM] Tryb pełny: Uruchamianie klasyfikacji dla wszystkich %d nieposortowanych zdjęć.", len(imageFiles)))
	}

	sendLog(fmt.Sprintf("[SYSTEM] Uruchamianie klasyfikacji dla %d zdjęć...", len(imageFiles)))
	sendProgress(10, "Inicjalizacja modelu OpenAI...")

	// Fetch all service names and map them to their database IDs
	services, err := s.db.ListServices(ctx)
	if err != nil {
		sendLog(fmt.Sprintf("[ERR] Błąd pobierania usług: %v", err))
		return
	}

	serviceMap := make(map[string]int64)
	var categories []string
	for _, svc := range services {
		serviceMap[svc.Name] = svc.ID
		categories = append(categories, svc.Name)
	}

	if len(categories) == 0 {
		sendLog("[ERR] Brak usług zdefiniowanych w bazie danych do dopasowania.")
		return
	}

	visionClient := vision.NewClient(s.cfg.OpenAIApiKey, s.cfg.AiVisionModel, s.cfg.VisionSortingPrompt)

	// Custom inline classification to stream logs for each file!
	for idx, file := range imageFiles {
		progressPct := 10 + int(float64(idx)/float64(len(imageFiles))*85.0)
		filename := file.Name()
		filePath := filepath.Join(screenshotsFolder, filename)
		sendProgress(progressPct, fmt.Sprintf("Klasyfikowanie obrazu %d/%d: %s...", idx+1, len(imageFiles), filename))

		matchedCategories, promptTokens, completionTokens, err := visionClient.ClassifyImage(ctx, filePath, categories)
		if err != nil {
			sendLog(fmt.Sprintf("  [ERR] Błąd klasyfikacji dla %s: %v", filename, err))
			continue
		}

		// Log OpenAI cost to DB
		cost := calculateOpenAICost(s.cfg.AiVisionModel, promptTokens, completionTokens)
		_ = s.db.LogCost(ctx, "Vision Sorting (Manual)", s.cfg.AiVisionModel, s.cfg.AiVisionModel, promptTokens, completionTokens, cost)

		if len(matchedCategories) == 0 {
			sendLog(fmt.Sprintf("  [OK] Zdjęcie %s odrzucone przez AI (nieprzydatne / niska pewność).", filename))
			continue
		}

		for _, cat := range matchedCategories {
			serviceID, ok := serviceMap[cat]
			if !ok {
				continue
			}

			// Check if photo is already classified for this service
			exists, _ := s.db.PhotoExistsForService(ctx, serviceID, filePath)
			if exists {
				sendLog(fmt.Sprintf("  [OK] Zdjęcie %s było już dopasowane do %s. Pomijam.", filename, cat))
				continue
			}

			_, err = s.db.CreatePhoto(ctx, serviceID, filePath, "Client", "pending")
			if err != nil {
				sendLog(fmt.Sprintf("  [ERR] Błąd zapisu dla %s w usłudze %s: %v", filename, cat, err))
			} else {
				sendLog(fmt.Sprintf("  [OK] Zdjęcie %s dopasowane do usługi: %s.", filename, cat))
			}
		}
	}

	sendProgress(100, "Zakończono!")
	send("complete", "done")
}

func scanIsImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".heic"
}

func calculateOpenAICost(model string, promptTokens, completionTokens int) float64 {
	inputRate := 0.00000015  // gpt-4o-mini default ($0.15 per 1M)
	outputRate := 0.00000060 // gpt-4o-mini default ($0.60 per 1M)

	if strings.Contains(strings.ToLower(model), "gpt-4o") && !strings.Contains(strings.ToLower(model), "mini") {
		inputRate = 0.0000025
		outputRate = 0.000010
	}
	return (float64(promptTokens) * inputRate) + (float64(completionTokens) * outputRate)
}
