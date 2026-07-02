package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Pepegakac123/photo-etl/internal/config"
	"github.com/Pepegakac123/photo-etl/internal/gallery"
	"github.com/Pepegakac123/photo-etl/internal/generator"
	"github.com/Pepegakac123/photo-etl/internal/ingest"
	"github.com/Pepegakac123/photo-etl/internal/stock"
	"github.com/Pepegakac123/photo-etl/internal/storage"
	"github.com/Pepegakac123/photo-etl/internal/translate"
	"github.com/Pepegakac123/photo-etl/internal/ui"
	"github.com/Pepegakac123/photo-etl/internal/version"
)

type translateWrapper struct{}

func (tw translateWrapper) Translate(ctx context.Context, text, fromLang, toLang string) (string, error) {
	return translate.Translate(ctx, text, fromLang, toLang)
}

func main() {
	if err := run(); err != nil {
		log.Printf("BŁĄD KRYTYCZNY: %v", err)
		if os.Getenv("NO_PAUSE") == "" && runtime.GOOS == "windows" {
			fmt.Println("\nNaciśnij klawisz Enter, aby zamknąć...")
			var input string
			fmt.Scanln(&input)
		}
		os.Exit(1)
	}
}

func run() error {
	defaultConfig, _ := config.GetDefaultConfigPath()
	configPath := flag.String("config", defaultConfig, "path to config file")
	clientDir := flag.String("client", "", "path to client directory to process")
	port := flag.Int("port", 8080, "port to run HTTP server on")
	versionFlag := flag.Bool("version", false, "print version info and exit")
	updateFlag := flag.Bool("update", false, "check for updates and install if available")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("Overflow Photo ETL version %s\n", version.CurrentVersion)
		return nil
	}

	if *updateFlag {
		fmt.Print("Sprawdzanie aktualizacji... ")
		newerReleases, err := version.CheckForUpdates("Pepegakac123/photo-etl")
		if err != nil {
			return fmt.Errorf("błąd podczas sprawdzania aktualizacji: %w", err)
		}

		if len(newerReleases) == 0 {
			fmt.Println("Masz już najnowszą wersję.")
			return nil
		}

		latestRelease := newerReleases[len(newerReleases)-1]
		fmt.Printf("\nDostępna jest nowa wersja: %s (Twoja to %s)\n", latestRelease.GetTagName(), version.CurrentVersion)

		var cumulativeChangelog strings.Builder
		for _, release := range newerReleases {
			if release.GetBody() != "" {
				cumulativeChangelog.WriteString(fmt.Sprintf("\n--- Zmiany w wersji %s ---\n", release.GetTagName()))
				cumulativeChangelog.WriteString(release.GetBody() + "\n")
			}
		}

		if cumulativeChangelog.Len() > 0 {
			fmt.Println(cumulativeChangelog.String())
			fmt.Println("---")
		}

		fmt.Println("Pobieranie i instalowanie aktualizacji...")
		if err := version.PerformUpdate(latestRelease); err != nil {
			return fmt.Errorf("błąd podczas aktualizacji: %w", err)
		}

		fmt.Println("Aktualizacja zakończona pomyślnie! Uruchom program ponownie.")
		return nil
	}

	// Ensure binary has correct name (photo-etl)
	if err := version.EnsureBinaryName("photo-etl"); err != nil {
		log.Printf("Nie udało się upewnić o nazwie pliku binarnego: %v", err)
	}
	version.CleanupOldBinary()

	var absClientDir string
	var db *storage.DB

	if *clientDir != "" {
		var err error
		absClientDir, err = filepath.Abs(*clientDir)
		if err != nil {
			return fmt.Errorf("invalid client directory: %w", err)
		}

		// Initialize database inside the client directory (short-lived project DB)
		dbPath := filepath.Join(absClientDir, "photo_etl.db")
		log.Printf("Initializing project SQLite database at: %s", dbPath)
		db, err = storage.InitDB(dbPath)
		if err != nil {
			return fmt.Errorf("error initializing database: %w", err)
		}
	} else {
		log.Println("No client directory specified at startup. You can load one from the UI homepage.")
	}

	// 1. Load config
	log.Printf("Starting Overflow Photo ETL...")
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	ctx := context.Background()

	// 3. Scan & Ingest client directory if DB is empty
	if db != nil {
		services, err := db.ListServices(ctx)
		if err != nil {
			return fmt.Errorf("failed to query services: %w", err)
		}

		if len(services) == 0 {
			log.Println("Database is empty. Running directory scanner...")
			scanner := ingest.NewScanner(db)
			res, err := scanner.Scan(ctx, absClientDir)
			if err != nil {
				return fmt.Errorf("failed to scan client directory: %w", err)
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
		return fmt.Errorf("failed to parse templates: %w", err)
	}

	addr := fmt.Sprintf("localhost:%d", *port)
	log.Printf("==================================================")
	log.Printf(" Overflow Photo ETL Supervision UI is ready!")
	log.Printf(" Open http://%s in your web browser.", addr)
	log.Printf(" Press Ctrl+C to stop the server.")
	log.Printf("==================================================")

	if err := http.ListenAndServe(addr, srv); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}
