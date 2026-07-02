package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// GetDefaultConfigPath returns the cross-platform default config file path in the user's home directory.
func GetDefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml", nil // fallback to local directory
	}

	configDir := filepath.Join(home, ".config", "photo-etl")
	_ = os.MkdirAll(configDir, 0755)
	return filepath.Join(configDir, "config.yaml"), nil
}

type Config struct {
	TargetPhotosPerService    int    `yaml:"target_photos_per_service"`
	LocalGalleryPath          string `yaml:"local_gallery_path"`
	ExportDir                 string `yaml:"export_dir"`
	GopressCmdPath            string `yaml:"gopress_cmd_path"`
	ConcurrencyLimit          int    `yaml:"concurrency_limit"`
	OpenAIApiKey              string `yaml:"openai_api_key"`
	AiVisionModel             string `yaml:"ai_vision_model"`
	EnvatoApiToken            string `yaml:"envato_api_token"`
	UnsplashAccessKey         string `yaml:"unsplash_access_key"`
	NanoBananaKey             string `yaml:"nano_banana_key"`
	VisionSortingPrompt       string `yaml:"vision_sorting_prompt"`
	ImageGenerationBasePrompt string `yaml:"image_generation_base_prompt"`
	GopressUpload             bool   `yaml:"gopress_upload"`
	GopressWpDomain           string `yaml:"gopress_wp_domain"`
	GopressWpUser             string `yaml:"gopress_wp_user"`
	GopressWpSecret           string `yaml:"gopress_wp_secret"`
	GopressFbToken            string `yaml:"gopress_wp_fb_token"`
}

// LoadConfig loads YAML configuration from path, overrides with environment variables,
// and sets default values for optional parameters.
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		TargetPhotosPerService: 5,
		LocalGalleryPath:       "/home/kacper/GoogleDrive/overflow-praca/Galeria Zdjęć Usługowych",
		ConcurrencyLimit:       5,
		AiVisionModel:          "gpt-4o-mini",
		VisionSortingPrompt:    "Jesteś ekspertem budowlanym. Przypisz zdjęcie do jednej z podanych kategorii. Jeśli zdjęcie nie pasuje do żadnej kategorii lub przedstawia śmieci, zwróć 'REJECT'.",
		ImageGenerationBasePrompt: "Zdjęcie musi wyglądać jak zrobione amatorsko, telefonem komórkowym, naturalne oświetlenie na budowie. Złota zasada: brak widocznych twarzy, brak logotypów, brak napisów. Styl surowy i realistyczny.",
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config file: %w", err)
			}
		} else if os.IsNotExist(err) {
			// Save default settings file
			defaultData, err := yaml.Marshal(cfg)
			if err == nil {
				_ = os.MkdirAll(filepath.Dir(path), 0755)
				_ = os.WriteFile(path, defaultData, 0644)
			}
		} else {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Override with environment variables
	if val := os.Getenv("TARGET_PHOTOS_PER_SERVICE"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.TargetPhotosPerService = i
		}
	}
	if val := os.Getenv("LOCAL_GALLERY_PATH"); val != "" {
		cfg.LocalGalleryPath = val
	}
	if val := os.Getenv("EXPORT_DIR"); val != "" {
		cfg.ExportDir = val
	}
	if val := os.Getenv("GOPRESS_CMD_PATH"); val != "" {
		cfg.GopressCmdPath = val
	}
	if val := os.Getenv("CONCURRENCY_LIMIT"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			cfg.ConcurrencyLimit = i
		}
	}
	if val := os.Getenv("OPENAI_API_KEY"); val != "" {
		cfg.OpenAIApiKey = val
	}
	if val := os.Getenv("AI_VISION_MODEL"); val != "" {
		cfg.AiVisionModel = val
	}
	if val := os.Getenv("ENVATO_API_TOKEN"); val != "" {
		cfg.EnvatoApiToken = val
	}
	if val := os.Getenv("NANO_BANANA_KEY"); val != "" {
		cfg.NanoBananaKey = val
	}
	if val := os.Getenv("VISION_SORTING_PROMPT"); val != "" {
		cfg.VisionSortingPrompt = val
	}
	if val := os.Getenv("IMAGE_GENERATION_BASE_PROMPT"); val != "" {
		cfg.ImageGenerationBasePrompt = val
	}
	if val := os.Getenv("GOPRESS_UPLOAD"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.GopressUpload = b
		}
	}
	if val := os.Getenv("GOPRESS_WP_DOMAIN"); val != "" {
		cfg.GopressWpDomain = val
	}
	if val := os.Getenv("GOPRESS_WP_USER"); val != "" {
		cfg.GopressWpUser = val
	}
	if val := os.Getenv("GOPRESS_WP_SECRET"); val != "" {
		cfg.GopressWpSecret = val
	}
	if val := os.Getenv("GOPRESS_WP_FB_TOKEN"); val != "" {
		cfg.GopressFbToken = val
	}

	return cfg, nil
}
