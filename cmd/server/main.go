package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Pepegakac123/photo-etl/internal/config"
	"github.com/Pepegakac123/photo-etl/internal/gallery"
	"github.com/Pepegakac123/photo-etl/internal/generator"
	"github.com/Pepegakac123/photo-etl/internal/ingest"
	"github.com/Pepegakac123/photo-etl/internal/stock"
	"github.com/Pepegakac123/photo-etl/internal/storage"
	"github.com/Pepegakac123/photo-etl/internal/translate"
	"github.com/Pepegakac123/photo-etl/internal/ui"
)

type translateWrapper struct{}

func (tw translateWrapper) Translate(ctx context.Context, text, fromLang, toLang string) (string, error) {
	return translate.Translate(ctx, text, fromLang, toLang)
}

func main() {
	defaultConfig, _ := config.GetDefaultConfigPath()
	configPath := flag.String("config", defaultConfig, "path to config file")
	clientDir := flag.String("client", "", "path to client directory to process")
	port := flag.Int("port", 8080, "port to run HTTP server on")
	flag.Parse()

	var absClientDir string
	var db *storage.DB

	if *clientDir != "" {
		var err error
		absClientDir, err = filepath.Abs(*clientDir)
		if err != nil {
			log.Fatalf("Invalid client directory: %v", err)
		}

		// Initialize database inside the client directory (short-lived project DB)
		dbPath := filepath.Join(absClientDir, "photo_etl.db")
		log.Printf("Initializing project SQLite database at: %s", dbPath)
		db, err = storage.InitDB(dbPath)
		if err != nil {
			log.Fatalf("Error initializing database: %v", err)
		}
	} else {
		log.Println("No client directory specified at startup. You can load one from the UI homepage.")
	}

	// 1. Load config
	log.Printf("Starting Overflow Photo ETL...")
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	ctx := context.Background()

	// 3. Scan & Ingest client directory if DB is empty
	if db != nil {
		services, err := db.ListServices(ctx)
		if err != nil {
			log.Fatalf("Failed to query services: %v", err)
		}

		if len(services) == 0 {
			log.Println("Database is empty. Running directory scanner...")
			scanner := ingest.NewScanner(db)
			res, err := scanner.Scan(ctx, absClientDir)
			if err != nil {
				log.Fatalf("Failed to scan client directory: %v", err)
			}
			log.Printf("Ingestion complete. Registered %d services.", len(res.ServicesAdded))
		} else {
			log.Printf("Resuming existing project. Found %d registered services in DB.", len(services))
		}
	}

	// 5. Initialize clients and services for the web server
	trans := translateWrapper{}
	galleryService := gallery.NewService(db, trans, cfg.LocalGalleryPath)

	// Index local gallery at startup in the background if path is set and valid
	if db != nil && cfg.LocalGalleryPath != "" {
		if _, err := os.Stat(cfg.LocalGalleryPath); err == nil {
			log.Printf("Indexing local gallery in background at: %s", cfg.LocalGalleryPath)
			go func() {
				if err := galleryService.IndexGallery(context.Background()); err != nil {
					log.Printf("Error indexing gallery in background: %v", err)
				} else {
					log.Printf("Local gallery background indexing complete!")
				}
			}()
		} else {
			log.Printf("WARNING: Local gallery path does not exist: %s. Indexing skipped.", cfg.LocalGalleryPath)
		}
	}

	bananaClient := generator.NewBananaClient(cfg.NanoBananaKey, cfg.ImageGenerationBasePrompt)
	envatoClient := stock.NewEnvatoClient(cfg.EnvatoApiToken)

	// 6. Initialize and start Web Server
	srv := ui.NewServer(db, cfg, *configPath, galleryService, bananaClient, envatoClient, absClientDir)
	
	// Parse templates
	if err := srv.ParseTemplates(); err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	addr := fmt.Sprintf("localhost:%d", *port)
	log.Printf("==================================================")
	log.Printf(" Overflow Photo ETL Supervision UI is ready!")
	log.Printf(" Open http://%s in your web browser.", addr)
	log.Printf(" Press Ctrl+C to stop the server.")
	log.Printf("==================================================")

	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
