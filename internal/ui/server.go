package ui

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Pepegakac123/photo-etl/internal/config"
	"github.com/Pepegakac123/photo-etl/internal/gallery"
	"github.com/Pepegakac123/photo-etl/internal/generator"
	"github.com/Pepegakac123/photo-etl/internal/stock"
	"github.com/Pepegakac123/photo-etl/internal/storage"
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
	s.mux.HandleFunc("POST /services/{id}/stock/search", s.handleStockSearch)
	s.mux.HandleFunc("POST /services/{id}/generate", s.handleGenerateImage)
	s.mux.HandleFunc("POST /photos/{id}/approve", s.handleApprovePhoto)
	s.mux.HandleFunc("POST /photos/{id}/reject", s.handleRejectPhoto)
	s.mux.HandleFunc("POST /photos/add", s.handleAddPhoto)
	s.mux.HandleFunc("GET /local-media", s.handleLocalMedia)
	s.mux.HandleFunc("POST /export", s.handleExport)
	s.mux.HandleFunc("GET /settings", s.handleSettings)
	s.mux.HandleFunc("POST /settings/test/gallery", s.handleTestGallery)
	s.mux.HandleFunc("POST /settings/test/openai", s.handleTestOpenAI)
	s.mux.HandleFunc("POST /settings/test/gemini", s.handleTestGemini)
	s.mux.HandleFunc("POST /settings/test/envato", s.handleTestEnvato)
	s.mux.HandleFunc("POST /settings/costs/clear", s.handleClearCosts)
	s.mux.HandleFunc("POST /client/select", s.handleClientSelect)
	s.mux.HandleFunc("GET /client/change", s.handleClientChange)
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
	if s.clientDir != "" {
		clientName = filepath.Base(s.clientDir)
	}

	data := map[string]interface{}{
		"Services":   sidebarData,
		"ClientDir":  s.clientDir,
		"ClientName": clientName,
	}

	err = s.tmpl.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		log.Printf("Template rendering error: %v", err)
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

	data := map[string]interface{}{
		"Service":       svc,
		"Photos":        activePhotos,
		"ApprovedCount": approvedCount,
		"RequiredCount": s.cfg.TargetPhotosPerService,
		"PendingCount":  pendingCount,
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

	// 4. Render the OOB AI results reset if adding an AI photo
	if r.URL.Query().Get("source") == "AI" {
		_, _ = w.Write([]byte(`
			<div id="ai-gen-results" hx-swap-oob="true" class="grid grid-cols-2 gap-4">
				<p class="text-sm text-gray-500 col-span-2">Kliknij przycisk powyżej, aby wygenerować zdjęcie za pomocą modelu Imagen.</p>
			</div>
		`))
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
		// limit previews in folder to 6
		if len(filteredPhotos) > 6 {
			filteredPhotos = filteredPhotos[:6]
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
	}

	err = s.tmpl.ExecuteTemplate(w, "gallery_results.html", data)
	if err != nil {
		log.Printf("Template gallery_results render error: %v", err)
	}
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
		http.Error(w, "Query term is required", http.StatusBadRequest)
		return
	}

	photos, err := s.envatoClient.SearchPhotos(ctx, term, 1, 10)
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
		"ServiceID: ": id,
		"ServiceID":   id,
		"Photos":      filteredPhotos,
	}

	err = s.tmpl.ExecuteTemplate(w, "stock_results.html", data)
	if err != nil {
		log.Printf("Template stock_results render error: %v", err)
	}
}

func (s *Server) handleGenerateImage(w http.ResponseWriter, r *http.Request) {
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

	// Generate description if empty using a quick translation/fallback
	desc := svc.ContextDescription
	if desc == "" {
		desc = fmt.Sprintf("Prace remontowo-budowlane w zakresie: %s", svc.Name)
	}

	// Output generated file to a temporary directory in workspace
	tempDir := filepath.Join(s.clientDir, ".temp")
	_ = os.MkdirAll(tempDir, 0755)

	filename := fmt.Sprintf("generated_%d_%d.png", id, os.Getpid())
	outputPath := filepath.Join(tempDir, filename)

	err = s.bananaClient.GenerateImage(ctx, svc.Name, "Niemcy", desc, outputPath)
	
	var data map[string]interface{}
	if err != nil {
		data = map[string]interface{}{
			"ServiceID": id,
			"Error":     err.Error(),
		}
	} else {
		// Log cost of image generation
		modelName := s.bananaClient.Model()
		cost := 0.067 // default for gemini-3.1-flash-image
		if modelName == "gemini-3-pro-image" {
			cost = 0.134
		}
		_ = s.db.LogCost(ctx, svc.Name, "image_generation", modelName, 0, 0, cost)

		data = map[string]interface{}{
			"ServiceID": id,
			"FilePath":  outputPath,
		}
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
	`, copyCount, s.cfg.ExportDir, gopressOutput)))
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
