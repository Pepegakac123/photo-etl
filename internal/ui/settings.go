package ui

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Pepegakac123/photo-etl/internal/vision"
)

// Helper to mask sensitive keys
func maskKey(key string) string {
	if len(key) <= 8 {
		return "..."
	}
	return key[len(key)-6:]
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	costs, err := s.db.ListCosts(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load cost statistics: %v", err), http.StatusInternalServerError)
		return
	}

	totalCosts, err := s.db.GetTotalCosts(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to calculate total costs: %v", err), http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Config":               s.cfg,
		"MaskedOpenAIKey":      maskKey(s.cfg.OpenAIApiKey),
		"MaskedNanoBananaKey":  maskKey(s.cfg.NanoBananaKey),
		"MaskedEnvatoToken":    maskKey(s.cfg.EnvatoApiToken),
		"Costs":                costs,
		"TotalCosts":           totalCosts,
	}

	err = s.tmpl.ExecuteTemplate(w, "settings", data)
	if err != nil {
		log.Printf("Failed to render settings template: %v", err)
	}
}

func renderTestResult(w http.ResponseWriter, success bool, message string) {
	w.Header().Set("Content-Type", "text/html")
	var class string
	var icon string
	if success {
		class = "bg-emerald-500/10 border-emerald-500/20 text-emerald-400"
		icon = `<svg class="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>`
	} else {
		class = "bg-rose-500/10 border-rose-500/20 text-rose-400"
		icon = `<svg class="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>`
	}

	fmt.Fprintf(w, `<div class="flex items-center gap-3 p-3 rounded-lg border %s text-xs transition-all duration-300">%s <span class="font-medium">%s</span></div>`, class, icon, message)
}

func (s *Server) handleTestGallery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := s.cfg.LocalGalleryPath
	if path == "" {
		renderTestResult(w, false, "Local gallery path is not configured")
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		renderTestResult(w, false, fmt.Sprintf("Directory does not exist or is not readable: %v", err))
		return
	}
	if !info.IsDir() {
		renderTestResult(w, false, "Specified path exists but is not a directory")
		return
	}

	// Count gallery folders from sqlite db
	folders, err := s.db.ListGalleryFolders(r.Context())
	if err != nil {
		renderTestResult(w, true, fmt.Sprintf("Directory is accessible, but failed to list gallery folders in DB: %v", err))
		return
	}

	renderTestResult(w, true, fmt.Sprintf("Dostępny. Zaindeksowane foldery w DB: %d", len(folders)))
}

func (s *Server) handleTestOpenAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.cfg.OpenAIApiKey == "" {
		renderTestResult(w, false, "Klucz API OpenAI jest pusty")
		return
	}

	visionClient := vision.NewClient(s.cfg.OpenAIApiKey, s.cfg.AiVisionModel, s.cfg.VisionSortingPrompt)
	err := visionClient.TestConnection(r.Context())
	if err != nil {
		renderTestResult(w, false, fmt.Sprintf("Błąd połączenia: %v", err))
		return
	}

	renderTestResult(w, true, "Połączenie udane. Model jest gotowy.")
}

func (s *Server) handleTestGemini(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.cfg.NanoBananaKey == "" {
		renderTestResult(w, false, "Klucz API Gemini (Nano Banana) jest pusty")
		return
	}

	err := s.bananaClient.TestConnection(r.Context())
	if err != nil {
		renderTestResult(w, false, fmt.Sprintf("Błąd połączenia: %v", err))
		return
	}

	renderTestResult(w, true, "Połączenie udane. Model jest gotowy.")
}

func (s *Server) handleTestEnvato(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.cfg.EnvatoApiToken == "" {
		renderTestResult(w, false, "Token API Envato jest pusty (aplikacja działa w trybie Mock)")
		return
	}

	err := s.envatoClient.TestConnection(r.Context())
	if err != nil {
		renderTestResult(w, false, fmt.Sprintf("Błąd połączenia: %v", err))
		return
	}

	renderTestResult(w, true, "Połączenie udane. Token jest ważny.")
}

func (s *Server) handleClearCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := s.db.ClearCosts(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to clear costs: %v", err), http.StatusInternalServerError)
		return
	}

	s.handleSettings(w, r)
}
