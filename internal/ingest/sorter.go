package ingest

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/Pepegakac123/photo-etl/internal/storage"
	"golang.org/x/sync/errgroup"
)

type ImageClassifier interface {
	ClassifyImage(ctx context.Context, imagePath string, categories []string) (string, error)
}

type Sorter struct {
	db          *storage.DB
	classifier  ImageClassifier
	concurrency int
}

func NewSorter(db *storage.DB, classifier ImageClassifier, concurrency int) *Sorter {
	if concurrency <= 0 {
		concurrency = 5
	}
	return &Sorter{
		db:          db,
		classifier:  classifier,
		concurrency: concurrency,
	}
}

// SortScreenshots scans the screenshotsFolder, classifies all images in it,
// and inserts the successfully classified images into the database.
func (s *Sorter) SortScreenshots(ctx context.Context, screenshotsFolder string) error {
	if screenshotsFolder == "" {
		return nil
	}

	files, err := os.ReadDir(screenshotsFolder)
	if err != nil {
		return err
	}

	// Fetch all service names and map them to their database IDs
	services, err := s.db.ListServices(ctx)
	if err != nil {
		return err
	}

	serviceMap := make(map[string]int64)
	var categories []string
	for _, svc := range services {
		serviceMap[svc.Name] = svc.ID
		categories = append(categories, svc.Name)
	}

	if len(categories) == 0 {
		return nil // No services to classify into
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.concurrency)

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !scanIsImage(file.Name()) {
			continue
		}

		filePath := filepath.Join(screenshotsFolder, file.Name())

		g.Go(func() error {
			category, err := s.classifier.ClassifyImage(ctx, filePath, categories)
			if err != nil {
				// We log the error but don't stop the whole pipeline unless desired.
				// However, if we propagate the error, the group fails.
				// Let's log it and continue so one bad image doesn't crash the ingest.
				log.Printf("Failed to classify image %s: %v", filePath, err)
				return nil
			}

			if category == "REJECT" {
				log.Printf("Image %s rejected by AI Vision", filePath)
				return nil
			}

			serviceID, ok := serviceMap[category]
			if !ok {
				log.Printf("Classification returned unknown category %q for image %s", category, filePath)
				return nil
			}

			// Insert classified photo into DB as 'Client' source with 'pending' status
			_, err = s.db.CreatePhoto(ctx, serviceID, filePath, "Client", "pending")
			if err != nil {
				return err
			}

			log.Printf("Image %s classified to service %s (ID: %d) successfully", filePath, category, serviceID)
			return nil
		})
	}

	return g.Wait()
}
