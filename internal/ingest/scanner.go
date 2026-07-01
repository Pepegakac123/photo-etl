package ingest

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Pepegakac123/photo-etl/internal/storage"
)

type Scanner struct {
	db *storage.DB
}

func NewScanner(db *storage.DB) *Scanner {
	return &Scanner{db: db}
}

type ScanResult struct {
	ServicesAdded     []string
	ScreenshotsFolder string
}

// scanIsImage checks if a file extension is a supported image type (case-insensitive).
func scanIsImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	}
	return false
}

// Scan scans the clientDir, detects the screenshots folder, registers services in the database,
// and imports existing client photos.
func (s *Scanner) Scan(ctx context.Context, clientDir string) (*ScanResult, error) {
	entries, err := os.ReadDir(clientDir)
	if err != nil {
		return nil, err
	}

	result := &ScanResult{}
	var serviceDirs []fs.DirEntry

	// Step 1: Separate service folders and screenshots folder
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		nameLower := strings.ToLower(entry.Name())
		if strings.Contains(nameLower, "whatsapp") || strings.Contains(nameLower, "zrzuty") {
			result.ScreenshotsFolder = filepath.Join(clientDir, entry.Name())
		} else {
			serviceDirs = append(serviceDirs, entry)
		}
	}

	// Step 2: Process each service folder
	for _, dir := range serviceDirs {
		serviceName := dir.Name()
		servicePath := filepath.Join(clientDir, serviceName)

		// Create service in DB
		serviceID, err := s.db.CreateService(ctx, serviceName)
		if err != nil {
			return nil, err
		}
		result.ServicesAdded = append(result.ServicesAdded, serviceName)

		// Scan photos in the service folder
		files, err := os.ReadDir(servicePath)
		if err != nil {
			return nil, err
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}
			if scanIsImage(file.Name()) {
				filePath := filepath.Join(servicePath, file.Name())
				// Existing files are imported as 'Client' source and 'approved' status
				_, err := s.db.CreatePhoto(ctx, serviceID, filePath, "Client", "approved")
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return result, nil
}
