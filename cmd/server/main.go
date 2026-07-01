package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/Pepegakac123/photo-etl/internal/config"
	"github.com/Pepegakac123/photo-etl/internal/storage"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	log.Println("Starting Overflow Photo ETL server...")

	// Load configuration
	log.Printf("Loading configuration from: %s", *configPath)
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	log.Printf("Configuration loaded successfully:")
	log.Printf(" - Target Photos Per Service: %d", cfg.TargetPhotosPerService)
	log.Printf(" - Local Gallery Path: %s", cfg.LocalGalleryPath)
	log.Printf(" - Concurrency Limit: %d", cfg.ConcurrencyLimit)

	// Initialize database
	dbPath := ":memory:"
	log.Printf("Initializing database at: %s", dbPath)
	db, err := storage.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		} else {
			log.Println("Database connection closed cleanly.")
		}
	}()

	// Ping database
	if err := db.Ping(); err != nil {
		log.Fatalf("Error pinging database: %v", err)
	}
	log.Println("Database connection verified and schema checked successfully.")

	fmt.Println("Initialization check passed successfully!")
}
