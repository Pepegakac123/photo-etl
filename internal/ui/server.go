package ui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sort"
	"strings"
	"sync"

	"github.com/Pepegakac123/photo-etl/internal/config"
	"github.com/Pepegakac123/photo-etl/internal/gallery"
	"github.com/Pepegakac123/photo-etl/internal/generator"
	"github.com/Pepegakac123/photo-etl/internal/stock"
	"github.com/Pepegakac123/photo-etl/internal/storage"
	"github.com/Pepegakac123/photo-etl/internal/translate"
)

type Server struct {
	db             *storage.DB
	cfg            *config.Config
	configPath     string
	galleryService *gallery.Service
	bananaClient   *generator.BananaClient
	envatoClient   *stock.EnvatoClient
	clientDir      string
	templatesDir   string
	tmpl           *template.Template
	mux            *http.ServeMux
}

func NewServer(db *storage.DB, cfg *config.Config, configPath string, gs *gallery.Service, bc *generator.BananaClient, ec *stock.EnvatoClient, clientDir string) *Server {
	s := &Server{
		db:             db,
		cfg:            cfg,
		configPath:     configPath,
		galleryService: gs,
		bananaClient:   bc,
		envatoClient:   ec,
		clientDir:      clientDir,
		templatesDir:   "views",
		mux:            http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) ParseTemplates() error {
	funcMap := template.FuncMap{
		"urlEscape": func(str string) string {
			return url.QueryEscape(str)
		},
		"baseName": func(str string) string {
			return filepath.Base(str)
		},
		"multiply": func(a float64, b float64) float64 {
			return a * b
		},
		"hasPrefix": func(s, prefix string) bool {
			return strings.HasPrefix(s, prefix)
		},
	}

	tmpl := template.New("").Funcs(funcMap)
	pattern := filepath.Join(s.templatesDir, "*.html")
	var err error
	s.tmpl, err = tmpl.ParseGlob(pattern)
	if err != nil {
		return fmt.Errorf("failed to parse templates in %s: %w", s.templatesDir, err)
	}
	return nil
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("GET /services/{id}", s.handleWorkspace)
	s.mux.HandleFunc("POST /services/{id}/gallery/search", s.handleGallerySearch)
	s.mux.HandleFunc("POST /services/{id}/gallery/autocomplete", s.handleGalleryAutocomplete)
	s.mux.HandleFunc("POST /services/{id}/gallery/associate-folder", s.handleGalleryAssociateFolder)
	s.mux.HandleFunc("POST /gallery/index", s.handleGalleryIndex)
	s.mux.HandleFunc("POST /services/{id}/photos/upload", s.handlePhotosUpload)
	s.mux.HandleFunc("POST /services/{id}/photos/reject-pending", s.handleRejectPendingPhotos)
	s.mux.HandleFunc("POST /services/{id}/stock/search", s.handleStockSearch)
	s.mux.HandleFunc("POST /services/{id}/generate", s.handleGenerateImage)
	s.mux.HandleFunc("POST /services/{id}/prompt/enhance", s.handleEnhancePrompt)
	s.mux.HandleFunc("POST /photos/{id}/approve", s.handleApprovePhoto)
	s.mux.HandleFunc("POST /photos/{id}/reject", s.handleRejectPhoto)
	s.mux.HandleFunc("POST /photos/add", s.handleAddPhoto)
	s.mux.HandleFunc("GET /local-media", s.handleLocalMedia)
	s.mux.HandleFunc("POST /export", s.handleExport)
	s.mux.HandleFunc("GET /export/stream", s.handleExportStream)
	s.mux.HandleFunc("GET /settings", s.handleSettings)
	s.mux.HandleFunc("POST /settings/test/gallery", s.handleTestGallery)
	s.mux.HandleFunc("POST /settings/test/openai", s.handleTestOpenAI)
	s.mux.HandleFunc("POST /settings/test/gemini", s.handleTestGemini)
	s.mux.HandleFunc("POST /settings/test/envato", s.handleTestEnvato)
	s.mux.HandleFunc("POST /settings/costs/clear", s.handleClearCosts)
	s.mux.HandleFunc("POST /client/select", s.handleClientSelect)
	s.mux.HandleFunc("GET /client/sort-screenshots/stream", s.handleSortScreenshotsStream)
	s.mux.HandleFunc("GET /client/change", s.handleClientChange)
	s.mux.HandleFunc("GET /client/unmatched-photos", s.handleUnmatchedPhotos)
	s.mux.HandleFunc("POST /photos/manual-match", s.handleManualMatch)
	s.mux.HandleFunc("POST /settings/save", s.handleSettingsSave)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse templates dynamically in development/testing if needed, or parse once.
	// We parse once in main, but let's make sure s.tmpl is not nil.
	if s.tmpl == nil {
		if err := s.ParseTemplates(); err != nil {
			http.Error(w, fmt.Sprintf("Template compilation error: %v", err), http.StatusInternalServerError)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

type serviceProgressView struct {
	ServiceID     int64
	ServiceName   string
	ApprovedCount int
	RequiredCount int
	PendingCount  int
	Oob           bool
}

func (s *Server) getSidebarData(ctx context.Context) ([]*serviceProgressView, error) {
	if s.db == nil {
		return nil, nil
	}
	progress, err := s.db.GetServiceProgress(ctx)
	if err != nil {
		return nil, err
	}

	var data []*serviceProgressView
	for _, p := range progress {
		data = append(data, &serviceProgressView{
			ServiceID:     p.ServiceID,
			ServiceName:   p.ServiceName,
			ApprovedCount: p.ApprovedCount,
			RequiredCount: s.cfg.TargetPhotosPerService,
			PendingCount:  p.PendingCount,
		})
	}
	sort.SliceStable(data, func(i, j int) bool {
		if data[i].PendingCount > 0 && data[j].PendingCount == 0 {
			return true
		}
		if data[i].PendingCount == 0 && data[j].PendingCount > 0 {
			return false
		}
		return strings.ToLower(data[i].ServiceName) < strings.ToLower(data[j].ServiceName)
	})
	return data, nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sidebarData, err := s.getSidebarData(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load index data: %v", err), http.StatusInternalServerError)
		return
	}

	clientName := ""
	var unmatchedCount int
	if s.clientDir != "" {
		clientName = filepath.Base(s.clientDir)
		unmatchedList, _ := s.getUnmatchedPhotosList(ctx)
		unmatchedCount = len(unmatchedList)
	}

	data := map[string]interface{}{
		"Services":       sidebarData,
		"ClientDir":      s.clientDir,
		"ClientName":     clientName,
		"UnmatchedCount": unmatchedCount,
	}

	err = s.tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		log.Printf("Template rendering error: %v", err)
	}
}

func (s *Server) getUnmatchedPhotosList(ctx context.Context) ([]string, error) {
	if s.clientDir == "" || s.db == nil {
		return nil, nil
	}

	// Detect screenshots folder
	var screenshotsFolder string
	entries, err := os.ReadDir(s.clientDir)
	if err != nil {
		return nil, err
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
		return nil, nil
	}

	files, err := os.ReadDir(screenshotsFolder)
	if err != nil {
		return nil, err
	}

	activePaths, err := s.db.GetActivePhotoPaths(ctx)
	if err != nil {
		return nil, err
	}

	var unmatched []string
	for _, file := range files {
		if !file.IsDir() && scanIsImage(file.Name()) {
			fullPath := filepath.Join(screenshotsFolder, file.Name())
			if !activePaths[fullPath] {
				unmatched = append(unmatched, fullPath)
			}
		}
	}

	return unmatched, nil
}

func (s *Server) handleUnmatchedPhotos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	photos, err := s.getUnmatchedPhotosList(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Błąd wczytywania zdjęć: %v", err), http.StatusInternalServerError)
		return
	}

	services, err := s.db.ListServices(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Błąd wczytywania usług: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert absolute paths to objects containing display details
	type viewPhoto struct {
		AbsolutePath string
		Filename     string
	}
	var viewPhotos []viewPhoto
	for _, p := range photos {
		viewPhotos = append(viewPhotos, viewPhoto{
			AbsolutePath: p,
			Filename:     filepath.Base(p),
		})
	}

	data := map[string]interface{}{
		"Photos":   viewPhotos,
		"Services": services,
	}

	err = s.tmpl.ExecuteTemplate(w, "unmatched_photos.html", data)
	if err != nil {
		log.Printf("Template unmatched_photos render error: %v", err)
	}
}

func (s *Server) handleManualMatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serviceIDStr := r.FormValue("service_id")
	filePath := r.FormValue("path")

	serviceID, err := strconv.ParseInt(serviceIDStr, 10, 64)
	if err != nil || filePath == "" {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<script>showToast("Nieprawidłowe parametry dopasowania.", "error");</script>`))
		return
	}

	svc, err := s.db.GetService(ctx, serviceID)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<script>showToast("Nie znaleziono wybranej usługi.", "error");</script>`))
		return
	}



	// Match the photo
	err = s.db.AddOrApprovePhoto(ctx, serviceID, filePath, "Client")
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<script>showToast("Błąd zapisu dopasowania: %v", "error");</script>`, err)))
		return
	}

	w.Header().Set("Content-Type", "text/html")
	var unmatchedCount int
	if s.clientDir != "" {
		unmatchedList, _ := s.getUnmatchedPhotosList(ctx)
		unmatchedCount = len(unmatchedList)
	}
	w.Write([]byte(fmt.Sprintf(`
		<div hx-swap-oob="beforeend:body"><script>showToast("Zdjęcie dopasowane pomyślnie do usługi: %s", "success");</script></div>
		<span id="unmatched-count-badge" hx-swap-oob="true" class="px-2 py-0.5 rounded bg-rose-500/10 text-rose-400 border border-rose-500/20 text-[10px] font-bold font-mono">
			%d
		</span>
	`, svc.Name, unmatchedCount)))

	// Update the sidebar service count out-of-band
	progress, err := s.getSingleServiceProgress(ctx, serviceID)
	if err == nil {
		progress.Oob = true
		_ = s.tmpl.ExecuteTemplate(w, "sidebar_button", progress)
	}
}

func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	svc, err := s.db.GetService(ctx, id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Service not found: %v", err), http.StatusNotFound)
		return
	}

	photos, err := s.db.ListPhotosByService(ctx, id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list photos: %v", err), http.StatusInternalServerError)
		return
	}

	// Filter out rejected photos for the workspace display
	var activePhotos []*storage.Photo
	var approvedCount int
	var pendingCount int
	for _, p := range photos {
		if p.Status != "rejected" {
			activePhotos = append(activePhotos, p)
			if p.Status == "approved" {
				approvedCount++
			} else if p.Status == "pending" {
				pendingCount++
			}
		}
	}

	// Translate service name to English for Envato Stock search field placeholder/default value
	translateText := svc.Name
	for _, sep := range []string{"_", " - "} {
		if parts := strings.SplitN(translateText, sep, 2); len(parts) > 1 {
			translateText = strings.TrimSpace(parts[0])
			break
		}
	}

	translatedName := translateText
	if tName, err := translate.Translate(ctx, translateText, "auto", "en"); err == nil && tName != "" {
		translatedName = tName
	}

	data := map[string]interface{}{
		"Service":       svc,
		"Photos":        activePhotos,
		"ApprovedCount": approvedCount,
		"RequiredCount": s.cfg.TargetPhotosPerService,
		"PendingCount":  pendingCount,
		"EnglishName":   translatedName,
	}

	err = s.tmpl.ExecuteTemplate(w, "workspace", data)
	if err != nil {
		log.Printf("Template workspace render error: %v", err)
	}
}

func (s *Server) getSingleServiceProgress(ctx context.Context, serviceID int64) (*serviceProgressView, error) {
	sidebarData, err := s.getSidebarData(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range sidebarData {
		if p.ServiceID == serviceID {
			return p, nil
		}
	}
	svc, err := s.db.GetService(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	return &serviceProgressView{
		ServiceID:     serviceID,
		ServiceName:   svc.Name,
		ApprovedCount: 0,
		RequiredCount: s.cfg.TargetPhotosPerService,
		PendingCount:  0,
	}, nil
}

func (s *Server) handleWorkspaceUpdate(w http.ResponseWriter, r *http.Request, serviceID int64) {
	ctx := r.Context()
	svc, err := s.db.GetService(ctx, serviceID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Service not found: %v", err), http.StatusNotFound)
		return
	}

	photos, err := s.db.ListPhotosByService(ctx, serviceID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list photos: %v", err), http.StatusInternalServerError)
		return
	}

	var activePhotos []*storage.Photo
	var approvedCount int
	var pendingCount int
	for _, p := range photos {
		if p.Status != "rejected" {
			activePhotos = append(activePhotos, p)
			if p.Status == "approved" {
				approvedCount++
			} else if p.Status == "pending" {
				pendingCount++
			}
		}
	}

	data := map[string]interface{}{
		"Service":       svc,
		"Photos":        activePhotos,
		"ApprovedCount": approvedCount,
		"RequiredCount": s.cfg.TargetPhotosPerService,
		"PendingCount":  pendingCount,
	}

	w.Header().Set("Content-Type", "text/html")

	// 1. Render the main project_photos list
	err = s.tmpl.ExecuteTemplate(w, "project_photos", data)
	if err != nil {
		log.Printf("Template project_photos render error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Render the OOB workspace status badge
	err = s.tmpl.ExecuteTemplate(w, "workspace_badge_oob", data)
	if err != nil {
		log.Printf("Template workspace_badge_oob render error: %v", err)
	}

	// 3. Render the OOB sidebar button progress update
	progress, err := s.getSingleServiceProgress(ctx, serviceID)
	if err == nil {
		progress.Oob = true
		err = s.tmpl.ExecuteTemplate(w, "sidebar_button", progress)
		if err != nil {
			log.Printf("Template sidebar_button render error: %v", err)
		}
	}

	// 4. Render the OOB unmatched count badge progress update
	if s.clientDir != "" {
		unmatchedList, _ := s.getUnmatchedPhotosList(ctx)
		unmatchedCount := len(unmatchedList)
		_, _ = fmt.Fprintf(w, `
			<span id="unmatched-count-badge" hx-swap-oob="true" class="px-2 py-0.5 rounded bg-rose-500/10 text-rose-400 border border-rose-500/20 text-[10px] font-bold font-mono">
				%d
			</span>
		`, unmatchedCount)
	}
}

func (s *Server) handleGallerySearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	svc, err := s.db.GetService(ctx, id)
	if err != nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	matches, isFallback, err := s.galleryService.MatchService(ctx, svc.Name)
	if err != nil {
		http.Error(w, fmt.Sprintf("Gallery match failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Limit to top 3 matches
	if len(matches) > 3 {
		matches = matches[:3]
	}

	// Query already added photos to filter duplicates
	existingPhotos, err := s.db.ListPhotosByService(ctx, id)
	addedPaths := make(map[string]bool)
	if err == nil {
		for _, p := range existingPhotos {
			if p.Status != "rejected" {
				addedPaths[p.FilePath] = true
			}
		}
	}

	// Fetch photos for each matched folder
	type folderMatch struct {
		Folder *storage.GalleryFolder
		Score  float64
		Photos []string
	}

	var results []folderMatch
	for _, m := range matches {
		photos, err := s.galleryService.ListPhotosInFolder(m.Folder.FolderPath)
		if err != nil {
			log.Printf("Failed to list photos in matched folder %s: %v", m.Folder.FolderName, err)
			continue
		}
		var filteredPhotos []string
		for _, p := range photos {
			if !addedPaths[p] {
				filteredPhotos = append(filteredPhotos, p)
			}
		}

		results = append(results, folderMatch{
			Folder: m.Folder,
			Score:  m.Score,
			Photos: filteredPhotos,
		})
	}

	data := map[string]interface{}{
		"ServiceID: ": id, // match structure helper
		"ServiceID":   id,
		"IsFallback":  isFallback,
		"Matches":     results,
		"RequiredCount": s.cfg.TargetPhotosPerService,
	}

	err = s.tmpl.ExecuteTemplate(w, "gallery_results.html", data)
	if err != nil {
		log.Printf("Template gallery_results render error: %v", err)
	}
}

func (s *Server) handleGalleryAutocomplete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	query := r.FormValue("manual-gallery-query")
	if len(strings.TrimSpace(query)) < 2 {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(""))
		return
	}

	folders, err := s.db.ListGalleryFolders(ctx)
	if err != nil {
		http.Error(w, "Failed to load folders", http.StatusInternalServerError)
		return
	}

	type matchItem struct {
		ID         int64
		FolderName string
		GermanName string
		Score      int
	}
	var matched []matchItem

	queryLower := strings.ToLower(strings.TrimSpace(query))
	for _, f := range folders {
		score := 0
		folderLower := strings.ToLower(f.FolderName)
		germanLower := strings.ToLower(f.GermanName)
		polishLower := strings.ToLower(f.PolishName)

		if strings.Contains(folderLower, queryLower) ||
			strings.Contains(germanLower, queryLower) ||
			strings.Contains(polishLower, queryLower) {
			score = 1
		}

		if score > 0 {
			matched = append(matched, matchItem{
				ID:         f.ID,
				FolderName: f.FolderName,
				GermanName: f.GermanName,
				Score:      score,
			})
		}
	}

	sort.SliceStable(matched, func(i, j int) bool {
		return strings.ToLower(matched[i].FolderName) < strings.ToLower(matched[j].FolderName)
	})

	if len(matched) > 15 {
		matched = matched[:15]
	}

	w.Header().Set("Content-Type", "text/html")
	if len(matched) == 0 {
		w.Write([]byte(`<div class="absolute left-0 right-0 mt-1 bg-[#0F1422] border border-[#23314B] rounded-xl p-3 text-xs text-gray-500 shadow-2xl z-50">Brak pasujących folderów</div>`))
		return
	}

	htmlContent := fmt.Sprintf(`<div class="absolute left-0 right-0 mt-1 bg-[#0F1422] border border-[#23314B] rounded-xl shadow-2xl overflow-hidden max-h-60 overflow-y-auto z-50 divide-y divide-[#23314B]/50 animate-fadeIn select-none">`)
	for _, item := range matched {
		escapedName := html.EscapeString(item.FolderName)
		escapedGerman := html.EscapeString(item.GermanName)
		htmlContent += fmt.Sprintf(`
			<button hx-post="/services/%d/gallery/associate-folder"
					hx-vals='{"folder_id": "%d"}'
					hx-target="#gallery-results"
					hx-indicator="#manual-gallery-indicator"
					onclick="setTimeout(clearAutocomplete, 100)"
					class="w-full text-left px-4 py-2.5 hover:bg-indigo-600/20 text-xs text-gray-200 hover:text-white transition flex items-center justify-between group active:scale-[0.99] outline-none">
				<span class="font-medium truncate pr-2">%s</span>
				<span class="text-[10px] text-gray-500 group-hover:text-indigo-400 font-mono shrink-0">%s</span>
			</button>
		`, id, item.ID, escapedName, escapedGerman)
	}
	htmlContent += `</div>`
	w.Write([]byte(htmlContent))
}

func (s *Server) handleGalleryAssociateFolder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	folderIDStr := r.FormValue("folder_id")
	folderID, err := strconv.ParseInt(folderIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid folder ID", http.StatusBadRequest)
		return
	}

	folder, err := s.db.GetGalleryFolder(ctx, folderID)
	if err != nil {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}

	existingPhotos, err := s.db.ListPhotosByService(ctx, id)
	addedPaths := make(map[string]bool)
	if err == nil {
		for _, p := range existingPhotos {
			if p.Status != "rejected" {
				addedPaths[p.FilePath] = true
			}
		}
	}

	photos, err := s.galleryService.ListPhotosInFolder(folder.FolderPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list photos: %v", err), http.StatusInternalServerError)
		return
	}

	var filteredPhotos []string
	for _, p := range photos {
		if !addedPaths[p] {
			filteredPhotos = append(filteredPhotos, p)
		}
	}



	type folderMatch struct {
		Folder *storage.GalleryFolder
		Score  float64
		Photos []string
	}

	results := []folderMatch{
		{
			Folder: folder,
			Score:  1.0,
			Photos: filteredPhotos,
		},
	}

	data := map[string]interface{}{
		"ServiceID: ": id,
		"ServiceID":   id,
		"IsFallback":  false,
		"Matches":     results,
		"RequiredCount": s.cfg.TargetPhotosPerService,
	}

	err = s.tmpl.ExecuteTemplate(w, "gallery_results.html", data)
	if err != nil {
		log.Printf("Template gallery_results render error: %v", err)
	}
}

func (s *Server) handleGalleryIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.cfg.LocalGalleryPath == "" {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<script>showToast("Ścieżka do galerii lokalnej nie jest ustawiona w konfiguracji.", "error");</script>`))
		return
	}

	err := s.galleryService.IndexGallery(ctx)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<script>showToast("Błąd indeksowania galerii: %v", "error");</script>`, err)))
		return
	}

	folders, err := s.db.ListGalleryFolders(ctx)
	count := 0
	if err == nil {
		count = len(folders)
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`<script>showToast("Pomyślnie zaktualizowano indeks galerii. Zaindeksowane foldery: %d", "success");</script>`, count)))
}

func (s *Server) handlePhotosUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	svc, err := s.db.GetService(ctx, id)
	if err != nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	err = r.ParseMultipartForm(50 << 20) // limit to 50MB
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<script>showToast("Błąd przesyłania plików: %v", "error");</script>`, err)))
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<script>showToast("Nie wybrano żadnych plików do przesłania.", "error");</script>`))
		return
	}

	destDir := filepath.Join(s.clientDir, svc.Name)
	_ = os.MkdirAll(destDir, 0755)

	uploadedCount := 0
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			log.Printf("Failed to open uploaded file %s: %v", fileHeader.Filename, err)
			continue
		}
		defer file.Close()

		filename := filepath.Base(fileHeader.Filename)
		finalPath := filepath.Join(destDir, filename)

		out, err := os.Create(finalPath)
		if err != nil {
			log.Printf("Failed to create file on disk %s: %v", finalPath, err)
			continue
		}
		defer out.Close()

		_, err = io.Copy(out, file)
		if err != nil {
			log.Printf("Failed to copy file content to %s: %v", finalPath, err)
			continue
		}

		_, err = s.db.CreatePhoto(ctx, id, finalPath, "Local", "approved")
		if err != nil {
			log.Printf("Failed to create photo record in DB: %v", err)
			continue
		}
		uploadedCount++
	}

	if uploadedCount > 0 {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<script>showToast("Pomyślnie dodano %d zdjęć z dysku.", "success");</script>`, uploadedCount)))
	} else {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<script>showToast("Nie udało się zapisać żadnego zdjęcia.", "error");</script>`))
	}

	s.handleWorkspaceUpdate(w, r, id)
}

func (s *Server) handleRejectPendingPhotos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	_, err = s.db.RejectPendingPhotos(ctx, id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to reject pending photos: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<script>showToast("Odrzucono wszystkie pozostałe zdjęcia oczekujące.", "success");</script>`))

	s.handleWorkspaceUpdate(w, r, id)
}

func (s *Server) handleStockSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	term := r.FormValue("term")
	if term == "" {
		term = r.URL.Query().Get("term")
	}
	if term == "" {
		http.Error(w, "Query term is required", http.StatusBadRequest)
		return
	}

	pageStr := r.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	photos, err := s.envatoClient.SearchPhotos(ctx, term, page, 10)
	if err != nil {
		http.Error(w, fmt.Sprintf("Stock search failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Query already added photos to filter duplicates
	existingPhotos, err := s.db.ListPhotosByService(ctx, id)
	addedPaths := make(map[string]bool)
	if err == nil {
		for _, p := range existingPhotos {
			if p.Status != "rejected" {
				addedPaths[p.FilePath] = true
			}
		}
	}

	var filteredPhotos []*stock.EnvatoPhoto
	for _, p := range photos {
		if !addedPaths[p.PreviewURL] {
			filteredPhotos = append(filteredPhotos, p)
		}
	}

	data := map[string]interface{}{
		"ServiceID: ":   id,
		"ServiceID":     id,
		"Photos":        filteredPhotos,
		"RequiredCount": s.cfg.TargetPhotosPerService,
		"Page":          page,
		"NextPage":      page + 1,
		"Term":          term,
	}

	err = s.tmpl.ExecuteTemplate(w, "stock_results.html", data)
	if err != nil {
		log.Printf("Template stock_results render error: %v", err)
	}
}

func (s *Server) handleEnhancePrompt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	svc, err := s.db.GetService(ctx, id)
	if err != nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	currentPrompt := r.FormValue("custom_prompt")
	if currentPrompt == "" {
		currentPrompt = svc.ContextDescription
	}
	if currentPrompt == "" {
		currentPrompt = svc.Name
	}

	if s.cfg.OpenAIApiKey == "" {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<script>showToast("Skonfiguruj najpierw klucz OpenAI w ustawieniach, aby ulepszać prompty.", "error");</script>`))
		return
	}

	enhancedPrompt, err := s.generateEnhancedPrompt(ctx, svc.Name, currentPrompt)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<script>showToast("Błąd ulepszania promptu: %v", "error");</script>`, err)))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	escapedPrompt := html.EscapeString(enhancedPrompt)
	textareaHTML := fmt.Sprintf(`
		<textarea id="custom_prompt" name="custom_prompt" rows="3" 
				  placeholder="Np. Prace dekarskie, układanie dachówek ceramicznych na dachu skośnym..."
				  class="w-full bg-[#0E1524] border border-[#23314B] rounded-lg px-3.5 py-2.5 text-xs text-white placeholder-gray-600 focus:outline-none focus:border-indigo-500 transition font-sans resize-y">%s</textarea>
	`, escapedPrompt)
	w.Write([]byte(textareaHTML))
}

func (s *Server) generateEnhancedPrompt(ctx context.Context, serviceName, contextDesc string) (string, error) {
	url := "https://api.openai.com/v1/chat/completions"

	systemPrompt := `Jesteś ekspertem od promptowania modeli generowania obrazów (takich jak Imagen/Midjourney/Gemini).
Twoim zadaniem jest przekształcić podaną nazwę usługi budowlano-remontowej oraz jej kontekst w szczegółowy, kreatywny i zróżnicowany prompt w języku angielskim.

BARDZO WAŻNE WYTYCZNE DOTYCZĄCE LUDZI I KOMPOZYCJI:
- Nie pokazuj całych postaci pracowników ani ich twarzy! Zamiast tego skup się na zbliżeniach (close-ups) na materiały, narzędzia lub wykonywaną pracę.
- Jeśli w ogóle pokazujesz człowieka, mogą to być wyłącznie części ciała (np. dłonie trzymające narzędzie, ręce układające dachówkę, buty stojące na rusztowaniu). Żadnych twarzy, żadnych pełnych sylwetek.
- Zdjęcie powinno wyglądać jak profesjonalne zdjęcie stockowe lub autentyczne zbliżenie zrobione na placu budowy w Niemczech.

Zadbaj o to, by każda wygenerowana scena była unikalna, dodając losowe kreatywne elementy (np. różne warunki oświetleniowe, kąty kamery, detale materiałowe, kolory).
Zwróć TYLKO i wyłącznie gotowy, czysty prompt w języku angielskim (maksymalnie 3-4 zdania). Nie dodawaj cudzysłowów ani żadnych słów wstępnych.`

	userPrompt := fmt.Sprintf("Nazwa usługi: %s\nKontekst: %s", serviceName, contextDesc)

	payload := map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 1.0,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.OpenAIApiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	if len(res.Choices) == 0 {
		return "", fmt.Errorf("empty response choices from OpenAI")
	}

	return strings.TrimSpace(res.Choices[0].Message.Content), nil
}

func (s *Server) handleGenerateImage(w http.ResponseWriter, r *http.Request) {
	if s.cfg.NanoBananaKey == "" {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<script>showToast("Skonfiguruj najpierw klucz Nano Banana w ustawieniach.", "error");</script>`))
		return
	}
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	svc, err := s.db.GetService(ctx, id)
	if err != nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	model := r.FormValue("model")
	if model == "" {
		model = "gemini-3.1-flash-image"
	}

	imageSize := r.FormValue("image_size")

	countStr := r.FormValue("count")
	count := 1
	if countStr != "" {
		if c, err := strconv.Atoi(countStr); err == nil && c >= 1 && c <= 5 {
			count = c
		}
	}

	// Generate description if empty using a quick translation/fallback
	customPrompt := r.FormValue("custom_prompt")
	var desc string
	if customPrompt != "" {
		if customPrompt != svc.ContextDescription {
			_ = s.db.UpdateServiceContextDescription(ctx, id, customPrompt)
		}
		desc = customPrompt
	} else {
		desc = svc.ContextDescription
		if desc == "" {
			desc = fmt.Sprintf("Prace remontowo-budowlane w zakresie: %s", svc.Name)
		}
	}

	// Output generated file to a temporary directory in workspace
	tempDir := filepath.Join(s.clientDir, ".temp")
	_ = os.MkdirAll(tempDir, 0755)

	type genResult struct {
		Path string
		Err  error
	}

	resChan := make(chan genResult, count)
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			filename := fmt.Sprintf("generated_%d_%d_%d.png", id, os.Getpid(), idx)
			outputPath := filepath.Join(tempDir, filename)
			err := s.bananaClient.GenerateImage(ctx, svc.Name, "Niemcy", desc, model, imageSize, outputPath)
			resChan <- genResult{Path: outputPath, Err: err}
		}(i)
	}

	wg.Wait()
	close(resChan)

	var generatedPaths []string
	var genErrors []error
	for res := range resChan {
		if res.Err != nil {
			genErrors = append(genErrors, res.Err)
		} else {
			generatedPaths = append(generatedPaths, res.Path)

			// Log cost of image generation based on selected model
			var cost float64
			switch model {
			case "gemini-3.1-flash-lite-image":
				cost = 0.015
			case "gemini-3-pro-image":
				cost = 0.134
			default:
				cost = 0.067
			}
			_ = s.db.LogCost(ctx, svc.Name, "image_generation", model, 0, 0, cost)
		}
	}

	var errStr string
	if len(generatedPaths) == 0 && len(genErrors) > 0 {
		errStr = genErrors[0].Error()
	}

	data := map[string]interface{}{
		"ServiceID":     id,
		"FilePaths":     generatedPaths,
		"Error":         errStr,
		"RequiredCount": s.cfg.TargetPhotosPerService,
	}

	err = s.tmpl.ExecuteTemplate(w, "ai_gen_results.html", data)
	if err != nil {
		log.Printf("Template ai_gen_results render error: %v", err)
	}
}

func (s *Server) handleApprovePhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid photo ID", http.StatusBadRequest)
		return
	}

	err = s.db.UpdatePhotoStatus(ctx, id, "approved")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to approve photo: %v", err), http.StatusInternalServerError)
		return
	}

	// Retrieve service ID from the photo to re-render workspace
	photo, err := s.db.GetPhoto(ctx, id)
	if err != nil {
		http.Error(w, "Failed to locate photo", http.StatusInternalServerError)
		return
	}

	s.handleWorkspaceUpdate(w, r, photo.ServiceID)
}

func (s *Server) handleRejectPhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid photo ID", http.StatusBadRequest)
		return
	}

	// Fetch photo to get file path and service ID
	photo, err := s.db.GetPhoto(ctx, id)
	if err == nil && photo != nil {
		// Delete file from filesystem if it was a copy (Stock or AI) inside the client directory
		if photo.FilePath != "" && strings.HasPrefix(photo.FilePath, s.clientDir) {
			_ = os.Remove(photo.FilePath)
		}
	}

	err = s.db.UpdatePhotoStatus(ctx, id, "rejected")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to reject photo: %v", err), http.StatusInternalServerError)
		return
	}

	// Set HTMX trigger header to reload search results automatically
	w.Header().Set("HX-Trigger", "reload-search")

	s.handleWorkspaceUpdate(w, r, photo.ServiceID)
}

func (s *Server) handleAddPhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serviceIDStr := r.URL.Query().Get("service_id")
	serviceID, err := strconv.ParseInt(serviceIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}



	srcPath := r.URL.Query().Get("path")
	source := r.URL.Query().Get("source")
	svc, err := s.db.GetService(ctx, serviceID)
	if err != nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	var finalPath string
	if source == "LocalGallery" {
		// Store the path directly, copy only during export to make UI instant
		finalPath = srcPath
	} else if source == "Stock" {
		// Store the preview URL directly, download only during export to make UI instant
		finalPath = srcPath
	} else if source == "AI" {
		// Copy local temp file immediately to persistent location (local copy is <1ms)
		destDir := filepath.Join(s.clientDir, svc.Name)
		_ = os.MkdirAll(destDir, 0755)
		filename := filepath.Base(srcPath)
		finalPath = filepath.Join(destDir, filename)
		if err := copyFile(srcPath, finalPath); err != nil {
			http.Error(w, fmt.Sprintf("Failed to import generated photo: %v", err), http.StatusInternalServerError)
			return
		}
		_ = os.Remove(srcPath)
	} else {
		http.Error(w, "Invalid photo source", http.StatusBadRequest)
		return
	}

	// Add photo to database as approved since user accepted it
	_, err = s.db.CreatePhoto(ctx, serviceID, finalPath, source, "approved")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to record photo in database: %v", err), http.StatusInternalServerError)
		return
	}

	s.handleWorkspaceUpdate(w, r, serviceID)
}

func (s *Server) handleLocalMedia(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	// Decode double URL-encoded paths (e.g. %2F -> /)
	if decoded, err := url.QueryUnescape(filePath); err == nil {
		filePath = decoded
	}
	log.Printf("[LOCAL-MEDIA] Serving file path: %q", filePath)
	if stat, err := os.Stat(filePath); err != nil {
		log.Printf("[LOCAL-MEDIA] File stat error for %q: %v", filePath, err)
	} else {
		log.Printf("[LOCAL-MEDIA] File stat OK: size=%d bytes, isDir=%v", stat.Size(), stat.IsDir())
	}
	http.ServeFile(w, r, filePath)
}

func (s *Server) getExportDir() string {
	if s.cfg.ExportDir != "" {
		return s.cfg.ExportDir
	}
	clientName := "default_client"
	if s.clientDir != "" {
		clientName = filepath.Base(s.clientDir)
	}
	return filepath.Join(os.TempDir(), "photo_etl_export_"+clientName)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	exportDir := s.getExportDir()

	// Clear export directory first
	_ = os.RemoveAll(exportDir)
	_ = os.MkdirAll(exportDir, 0755)

	services, err := s.db.ListServices(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to retrieve services: %v", err), http.StatusInternalServerError)
		return
	}

	// Validate minimum photo count for each service before exporting
	for _, svc := range services {
		photos, err := s.db.ListPhotosByService(ctx, svc.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to list photos for service %s: %v", svc.Name, err), http.StatusInternalServerError)
			return
		}
		approvedCount := 0
		for _, p := range photos {
			if p.Status == "approved" {
				approvedCount++
			}
		}
		if approvedCount < s.cfg.TargetPhotosPerService {
			http.Error(w, fmt.Sprintf("Usługa '%s' nie spełnia minimalnego limitu zdjęć (%d/%d zatwierdzonych).", svc.Name, approvedCount, s.cfg.TargetPhotosPerService), http.StatusBadRequest)
			return
		}
	}

	var copyCount int
	for _, svc := range services {
		photos, err := s.db.ListPhotosByService(ctx, svc.ID)
		if err != nil {
			continue
		}

		destServiceDir := filepath.Join(exportDir, svc.Name)
		_ = os.MkdirAll(destServiceDir, 0755)

		for _, p := range photos {
			if p.Status == "approved" {
				// Avoid query parameters in the filename for Stock URL paths
				filename := filepath.Base(p.FilePath)
				if p.Source == "Stock" {
					filename = fmt.Sprintf("stock_%d.jpg", p.ID)
				}
				destPath := filepath.Join(destServiceDir, filename)

				if p.Source == "Stock" {
					if err := downloadFile(p.FilePath, destPath); err != nil {
						log.Printf("[EXPORT] Failed to download stock photo from %s: %v", p.FilePath, err)
					} else {
						copyCount++
					}
				} else {
					if err := copyFile(p.FilePath, destPath); err != nil {
						log.Printf("[EXPORT] Failed to copy local photo %s: %v", p.FilePath, err)
					} else {
						copyCount++
					}
				}
			}
		}
	}

	log.Printf("Exported %d approved photos to %s", copyCount, exportDir)

	// Execute GoPress CLI if configured
	gopressOutput := ""
	if s.cfg.GopressCmdPath != "" {
		if _, err := os.Stat(s.cfg.GopressCmdPath); err == nil {
			args := []string{"-i", exportDir}
			if s.cfg.GopressUpload {
				args = append(args, "--upload")
				if s.cfg.GopressWpDomain != "" {
					args = append(args, "--wp-domain", s.cfg.GopressWpDomain)
				}
				if s.cfg.GopressWpUser != "" {
					args = append(args, "--wp-user", s.cfg.GopressWpUser)
				}
				if s.cfg.GopressWpSecret != "" {
					args = append(args, "--wp-secret", s.cfg.GopressWpSecret)
				}
				if s.cfg.GopressFbToken != "" {
					args = append(args, "--fb-token", s.cfg.GopressFbToken)
				}
			}
			log.Printf("Executing GoPress CLI at: %s %v", s.cfg.GopressCmdPath, args)
			cmd := exec.CommandContext(ctx, s.cfg.GopressCmdPath, args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				gopressOutput = fmt.Sprintf("GoPress error: %v. Output: %s", err, string(output))
			} else {
				gopressOutput = fmt.Sprintf("GoPress optimization complete:\n%s", string(output))
				if s.cfg.LocalGalleryPath != "" {
					s.mergeNonClientPhotosToGallery(ctx, exportDir, nil)
				}
			}
		} else {
			gopressOutput = "GoPress CLI executable not found at specified path."
		}
	} else {
		gopressOutput = "GoPress CLI not configured. Photos exported successfully."
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`
		<div class="h-full flex flex-col items-center justify-center text-center p-8 bg-[#090D16] glass border border-[#23314B] rounded-2xl glow">
			<div class="w-20 h-20 bg-emerald-500/10 border border-emerald-500/20 rounded-full flex items-center justify-center mb-6 glow">
				<svg class="w-10 h-10 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path>
				</svg>
			</div>
			<h2 class="text-xl font-bold text-white mb-2">Eksport zakończony sukcesem!</h2>
			<p class="text-sm text-gray-400 max-w-md mb-6">
				Wyeksportowano %d zdjęć do katalogu: <br><span class="font-mono text-indigo-300 text-xs break-all">%s</span>
			</p>
			<pre class="bg-[#0E1524] border border-[#23314B] rounded-xl p-4 text-xs font-mono text-left max-w-xl overflow-x-auto text-gray-300 w-full whitespace-pre-wrap">%s</pre>
		</div>
	`, copyCount, exportDir, gopressOutput)))
}

// Helpers
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func downloadFile(fileURL, dst string) error {
	resp, err := http.Get(fileURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func cleanFilename(title string) string {
	res := strings.ToLower(title)
	res = strings.ReplaceAll(res, " ", "_")
	// Keep only alphanumeric and underscores
	var clean []rune
	for _, r := range res {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			clean = append(clean, r)
		}
	}
	if len(clean) > 30 {
		clean = clean[:30]
	}
	return string(clean)
}

func (s *Server) handleExportStream(w http.ResponseWriter, r *http.Request) {
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

	sendLog("[SYSTEM] Uruchamianie procesu eksportu...")
	sendProgress(5, "Inicjalizacja katalogów...")

	exportDir := s.getExportDir()
	sendLog(fmt.Sprintf("[SYSTEM] Katalog docelowy eksportu: %s", exportDir))

	// Clear export directory first
	_ = os.RemoveAll(exportDir)
	_ = os.MkdirAll(exportDir, 0755)

	services, err := s.db.ListServices(ctx)
	if err != nil {
		sendLog(fmt.Sprintf("[ERR] Błąd pobierania usług z bazy: %v", err))
		return
	}

	if len(services) == 0 {
		sendLog("[ERR] Brak zarejestrowanych usług w projekcie.")
		return
	}

	// Validate minimum photo count for each service before exporting
	hasErrors := false
	for _, svc := range services {
		photos, err := s.db.ListPhotosByService(ctx, svc.ID)
		if err != nil {
			sendLog(fmt.Sprintf("[ERR] Błąd listowania zdjęć dla usługi %s: %v", svc.Name, err))
			hasErrors = true
			continue
		}
		approvedCount := 0
		for _, p := range photos {
			if p.Status == "approved" {
				approvedCount++
			}
		}
		if approvedCount < s.cfg.TargetPhotosPerService {
			sendLog(fmt.Sprintf("[ERR] Usługa '%s' nie spełnia minimalnego limitu zdjęć (%d/%d approved). Uzupełnij zdjęcia przed eksportem.", svc.Name, approvedCount, s.cfg.TargetPhotosPerService))
			hasErrors = true
		}
	}

	if hasErrors {
		sendLog("[ERR] Eksport przerwany: nie wszystkie usługi spełniają minimalny limit zdjęć.")
		sendProgress(0, "Błąd eksportu - brakujące zdjęcia")
		return
	}

	sendLog(fmt.Sprintf("[SYSTEM] Znaleziono %d zarejestrowanych usług w bazie danych.", len(services)))
	sendProgress(10, "Kopiowanie i pobieranie zdjęć...")

	totalSteps := len(services)
	var copyCount int

	for index, svc := range services {
		progressPct := 10 + int(float64(index)/float64(totalSteps)*60.0) // 10% to 70%
		sendProgress(progressPct, fmt.Sprintf("Przetwarzanie usługi: %s...", svc.Name))

		photos, err := s.db.ListPhotosByService(ctx, svc.ID)
		if err != nil {
			sendLog(fmt.Sprintf("  [ERR] Błąd listowania zdjęć dla usługi %s: %v", svc.Name, err))
			continue
		}

		destServiceDir := filepath.Join(exportDir, svc.Name)
		_ = os.MkdirAll(destServiceDir, 0755)

		approvedPhotos := 0
		for _, p := range photos {
			if p.Status == "approved" {
				approvedPhotos++
				// Avoid query parameters in the filename for Stock URL paths
				filename := filepath.Base(p.FilePath)
				if p.Source == "Stock" {
					filename = fmt.Sprintf("stock_%d.jpg", p.ID)
				}
				destPath := filepath.Join(destServiceDir, filename)

				if p.Source == "Stock" {
					if strings.Contains(p.FilePath, "envatousercontent") {
						sendLog(fmt.Sprintf("  -> Pobieranie oryginalnego zdjęcia z Envato: %s ...", filename))
					} else {
						sendLog(fmt.Sprintf("  -> Pobieranie zdjęcia stockowego (Unsplash mock): %s ...", filename))
					}
					if err := downloadFile(p.FilePath, destPath); err != nil {
						sendLog(fmt.Sprintf("  [ERR] Błąd pobierania z %s: %v", p.FilePath, err))
					} else {
						// Ensure Envato stock photos are at least 1920px wide
						if err := ensureMinimumWidth(destPath, 1920); err != nil {
							sendLog(fmt.Sprintf("  [SYSTEM] Ostrzeżenie: Błąd podbicia rozdzielczości do 1920px: %v", err))
						} else {
							sendLog(fmt.Sprintf("  [SYSTEM] Dopasowano rozdzielczość zdjęcia do min. 1920px."))
						}
						sendLog(fmt.Sprintf("  [OK] Pomyślnie pobrano: %s", filename))
						copyCount++
					}
				} else {
					sendLog(fmt.Sprintf("  -> Kopiowanie zdjęcia lokalnego: %s ...", filename))
					if err := copyFile(p.FilePath, destPath); err != nil {
						sendLog(fmt.Sprintf("  [ERR] Błąd kopiowania pliku %s: %v", p.FilePath, err))
					} else {
						sendLog(fmt.Sprintf("  [OK] Skopiowano do: %s", filename))
						copyCount++
					}
				}
			}
		}
		sendLog(fmt.Sprintf("[SYSTEM] Zakończono przetwarzanie usługi %s. Zatwierdzonych zdjęć: %d.", svc.Name, approvedPhotos))
	}

	sendLog(fmt.Sprintf("[SYSTEM] Eksport plików zakończony. Wyeksportowano łącznie %d zdjęć.", copyCount))
	sendProgress(70, "Uruchamianie optymalizacji GoPress...")

	// Execute GoPress CLI if configured
	if s.cfg.GopressCmdPath != "" {
		if _, err := os.Stat(s.cfg.GopressCmdPath); err == nil {
			args := []string{"-i", exportDir}
			if s.cfg.GopressUpload {
				args = append(args, "--upload")
				if s.cfg.GopressWpDomain != "" {
					args = append(args, "--wp-domain", s.cfg.GopressWpDomain)
				}
				if s.cfg.GopressWpUser != "" {
					args = append(args, "--wp-user", s.cfg.GopressWpUser)
				}
				if s.cfg.GopressWpSecret != "" {
					args = append(args, "--wp-secret", s.cfg.GopressWpSecret)
				}
				if s.cfg.GopressFbToken != "" {
					args = append(args, "--fb-token", s.cfg.GopressFbToken)
				}
			}
			sendLog(fmt.Sprintf("[SYSTEM] Uruchamianie GoPress CLI: %s %s", s.cfg.GopressCmdPath, strings.Join(args, " ")))
			sendProgress(75, "Optymalizacja obrazów w GoPress...")

			cmd := exec.CommandContext(ctx, s.cfg.GopressCmdPath, args...)

			stdoutPipe, err := cmd.StdoutPipe()
			if err != nil {
				sendLog(fmt.Sprintf("Błąd pipe stdout GoPress: %v", err))
			}
			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				sendLog(fmt.Sprintf("Błąd pipe stderr GoPress: %v", err))
			}

			if err := cmd.Start(); err != nil {
				sendLog(fmt.Sprintf("GoPress error: failed to start command: %v", err))
			} else {
				var wg sync.WaitGroup
				scanLog := func(r io.Reader) {
					defer wg.Done()
					scanner := bufio.NewScanner(r)
					for scanner.Scan() {
						line := scanner.Text()
						sendLog(line)
					}
				}

				if stdoutPipe != nil {
					wg.Add(1)
					go scanLog(stdoutPipe)
				}
				if stderrPipe != nil {
					wg.Add(1)
					go scanLog(stderrPipe)
				}

				wg.Wait()
				if err := cmd.Wait(); err != nil {
					sendLog(fmt.Sprintf("GoPress error: CLI exited with error: %v", err))
				} else {
					sendLog("[SYSTEM] GoPress CLI zakończył działanie pomyślnie.")
					if s.cfg.LocalGalleryPath != "" {
						sendLog("[SYSTEM] Synchronizacja przefiltrowanych zdjęć (bez WhatsApp) do galerii lokalnej...")
						s.mergeNonClientPhotosToGallery(ctx, exportDir, sendLog)
					}
				}
			}
		} else {
			sendLog(fmt.Sprintf("GoPress CLI executable not found at: %s", s.cfg.GopressCmdPath))
		}
	} else {
		sendLog("[SYSTEM] GoPress CLI not configured. Skipping GoPress upload/optimization step.")
	}

	sendProgress(100, "Zakończono!")
	send("complete", "done")
}

func ensureMinimumWidth(filePath string, minWidth int) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return err
	}

	if config.Width >= minWidth {
		return nil
	}

	// Seek to beginning
	_, _ = file.Seek(0, 0)

	img, _, err := image.Decode(file)
	if err != nil {
		return err
	}
	file.Close() // Close before recreating the file

	newHeight := (minWidth * config.Height) / config.Width
	newImg := image.NewRGBA(image.Rect(0, 0, minWidth, newHeight))

	// Scaling up using nearest neighbor interpolation
	for y := 0; y < newHeight; y++ {
		for x := 0; x < minWidth; x++ {
			srcX := (x * config.Width) / minWidth
			srcY := (y * config.Height) / minWidth
			newImg.Set(x, y, img.At(img.Bounds().Min.X+srcX, img.Bounds().Min.Y+srcY))
		}
	}

	outFile, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	if format == "png" {
		return png.Encode(outFile, newImg)
	}
	return jpeg.Encode(outFile, newImg, &jpeg.Options{Quality: 95})
}

func (s *Server) mergeNonClientPhotosToGallery(ctx context.Context, exportDir string, sendLog func(string)) {
	if s.cfg.LocalGalleryPath == "" {
		return
	}

	services, err := s.db.ListServices(ctx)
	if err != nil {
		return
	}

	for _, svc := range services {
		photos, err := s.db.ListPhotosByService(ctx, svc.ID)
		if err != nil {
			continue
		}

		for _, p := range photos {
			if p.Status == "approved" && p.Source != "Client" {
				filename := filepath.Base(p.FilePath)
				if p.Source == "Stock" {
					filename = fmt.Sprintf("stock_%d.jpg", p.ID)
				}
				
				optFilename := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".webp"
				optFilePath := filepath.Join(exportDir, svc.Name, optFilename)

				if _, err := os.Stat(optFilePath); err == nil {
					destDir := filepath.Join(s.cfg.LocalGalleryPath, svc.Name)
					_ = os.MkdirAll(destDir, 0755)
					destPath := filepath.Join(destDir, optFilename)
					
					if err := copyFile(optFilePath, destPath); err == nil {
						if sendLog != nil {
							sendLog(fmt.Sprintf("  [GALERIA] Skopiowano do galerii lokalnej: %s/%s", svc.Name, optFilename))
						} else {
							log.Printf("[GALERIA] Skopiowano do galerii lokalnej: %s/%s", svc.Name, optFilename)
						}
					}
				}
			}
		}
	}
}
