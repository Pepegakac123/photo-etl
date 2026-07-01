package ui

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

func (s *Server) handleSortScreenshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.clientDir == "" || s.db == nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<script>showToast("Najpierw wczytaj folder klienta.", "error");</script>`))
		return
	}

	if s.cfg.OpenAIApiKey == "" {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<script>showToast("Skonfiguruj klucz OpenAI w ustawieniach, aby uruchomić klasyfikację.", "error");</script>`))
		return
	}

	// Detect screenshots folder
	var screenshotsFolder string
	entries, err := os.ReadDir(s.clientDir)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<script>showToast("Błąd odczytu katalogu: %v", "error");</script>`, err)))
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
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<script>showToast("Nie znaleziono folderu WhatsApp ani zrzutów ekranu.", "error");</script>`))
		return
	}

	ctx := r.Context()
	visionClient := vision.NewClient(s.cfg.OpenAIApiKey, s.cfg.AiVisionModel, s.cfg.VisionSortingPrompt)
	sorter := ingest.NewSorter(s.db, visionClient, s.cfg.ConcurrencyLimit)
	
	err = sorter.SortScreenshots(ctx, screenshotsFolder)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<script>showToast("Błąd klasyfikacji: %v", "error");</script>`, err)))
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<script>showToast("Klasyfikacja AI zakończona pomyślnie!", "success"); setTimeout(() => { window.location.reload(); }, 1500);</script>`))
}
