package gallery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Pepegakac123/photo-etl/internal/storage"
)

type Translator interface {
	Translate(ctx context.Context, text, fromLang, toLang string) (string, error)
}

type Service struct {
	db          *storage.DB
	translator  Translator
	galleryPath string
}

func NewService(db *storage.DB, trans Translator, galleryPath string) *Service {
	return &Service{
		db:          db,
		translator:  trans,
		galleryPath: galleryPath,
	}
}

func (s *Service) SetDB(db *storage.DB) {
	s.db = db
}

func (s *Service) SetLocalGalleryPath(path string) {
	s.galleryPath = path
}

type MatchResult struct {
	Folder *storage.GalleryFolder
	Score  float64
}

// IndexGallery scans the local gallery path, parses subfolders, and stores them in the DB.
func (s *Service) IndexGallery(ctx context.Context) error {
	if s.galleryPath == "" {
		return fmt.Errorf("local gallery path is empty")
	}

	entries, err := os.ReadDir(s.galleryPath)
	if err != nil {
		return fmt.Errorf("failed to read local gallery directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		folderName := entry.Name()
		folderPath := filepath.Join(s.galleryPath, folderName)

		var germanName, polishName string
		parts := strings.SplitN(folderName, "_", 2)
		if len(parts) == 2 {
			germanName = strings.TrimSpace(parts[0])
			polishName = strings.TrimSpace(parts[1])
		} else {
			germanName = ""
			polishName = strings.TrimSpace(folderName)
		}

		_, err := s.db.CreateGalleryFolder(ctx, folderName, folderPath, germanName, polishName)
		if err != nil {
			return fmt.Errorf("failed to index gallery folder %s: %w", folderName, err)
		}
	}

	return nil
}

// MatchService finds the best matching gallery folders for a query service name.
func (s *Service) MatchService(ctx context.Context, query string) ([]*MatchResult, bool, error) {
	folders, err := s.db.ListGalleryFolders(ctx)
	if err != nil {
		return nil, false, err
	}

	// 1. Try Direct Matching
	var directResults []*MatchResult
	var maxDirectScore float64

	for _, f := range folders {
		score := maxSimilarity(query, f.FolderName, f.GermanName, f.PolishName)
		if score > maxDirectScore {
			maxDirectScore = score
		}
		directResults = append(directResults, &MatchResult{
			Folder: f,
			Score:  score,
		})
	}

	// Sort direct results descending
	slices.SortFunc(directResults, func(a, b *MatchResult) int {
		if b.Score > a.Score {
			return 1
		} else if b.Score < a.Score {
			return -1
		}
		return 0
	})

	// Threshold for direct match: if max score is >= 0.80, we return direct match results
	if maxDirectScore >= 0.80 {
		return directResults, false, nil
	}

	// 2. Translation Fallback (translate to Polish, match against polish_name)
	polishTranslation, err := s.translator.Translate(ctx, query, "auto", "pl")
	if err != nil {
		// Log error and fallback to direct results
		return directResults, false, fmt.Errorf("translation failed, fallback to direct search: %w", err)
	}

	var fallbackResults []*MatchResult
	for _, f := range folders {
		score := levenshteinSimilarity(polishTranslation, f.PolishName)
		fallbackResults = append(fallbackResults, &MatchResult{
			Folder: f,
			Score:  score,
		})
	}

	// Sort fallback results descending
	slices.SortFunc(fallbackResults, func(a, b *MatchResult) int {
		if b.Score > a.Score {
			return 1
		} else if b.Score < a.Score {
			return -1
		}
		return 0
	})

	return fallbackResults, true, nil
}

func maxSimilarity(query string, targets ...string) float64 {
	var maxScore float64
	for _, target := range targets {
		if target == "" {
			continue
		}
		score := levenshteinSimilarity(query, target)
		if score > maxScore {
			maxScore = score
		}
	}
	return maxScore
}

func levenshteinSimilarity(s1, s2 string) float64 {
	s1 = strings.ToLower(strings.TrimSpace(s1))
	s2 = strings.ToLower(strings.TrimSpace(s2))
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}
	dist := levenshteinDistance(s1, s2)
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}
	return 1.0 - float64(dist)/float64(maxLen)
}

func levenshteinDistance(s, t string) int {
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(s); i++ {
		for j := 1; j <= len(t); j++ {
			cost := 0
			if s[i-1] != t[j-1] {
				cost = 1
			}
			d[i][j] = minOfThree(
				d[i-1][j]+1,      // deletion
				d[i][j-1]+1,      // insertion
				d[i-1][j-1]+cost, // substitution
			)
		}
	}
	return d[len(s)][len(t)]
}

func minOfThree(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}

// ListPhotosInFolder lists all image files inside a given gallery folder path.
func (s *Service) ListPhotosInFolder(folderPath string) ([]string, error) {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read gallery folder: %w", err)
	}

	var photos []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if isImageFile(entry.Name()) {
			photos = append(photos, filepath.Join(folderPath, entry.Name()))
		}
	}
	return photos, nil
}

func isImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"
}

