package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	yamlData := `
target_photos_per_service: 5
local_gallery_path: "/home/kacper/GoogleDrive/overflow-praca/Galeria Zdjęć Usługowych"
export_dir: "/path/to/export"
gopress_cmd_path: "/path/to/gopress"
concurrency_limit: 5
openai_api_key: "test-openai-key"
ai_vision_model: "gpt-4o-mini"
envato_api_token: "test-envato-token"
nano_banana_key: "test-nano-banana-key"
vision_sorting_prompt: "Jesteś ekspertem budowlanym..."
image_generation_base_prompt: "Zdjęcie musi wyglądać jak zrobione amatorsko..."
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlData)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	cfg, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.TargetPhotosPerService != 5 {
		t.Errorf("expected TargetPhotosPerService 5, got %d", cfg.TargetPhotosPerService)
	}
	if cfg.LocalGalleryPath != "/home/kacper/GoogleDrive/overflow-praca/Galeria Zdjęć Usługowych" {
		t.Errorf("expected LocalGalleryPath match, got %q", cfg.LocalGalleryPath)
	}
	if cfg.ExportDir != "/path/to/export" {
		t.Errorf("expected ExportDir /path/to/export, got %q", cfg.ExportDir)
	}
	if cfg.GopressCmdPath != "/path/to/gopress" {
		t.Errorf("expected GopressCmdPath /path/to/gopress, got %q", cfg.GopressCmdPath)
	}
	if cfg.ConcurrencyLimit != 5 {
		t.Errorf("expected ConcurrencyLimit 5, got %d", cfg.ConcurrencyLimit)
	}
	if cfg.OpenAIApiKey != "test-openai-key" {
		t.Errorf("expected OpenAIApiKey 'test-openai-key', got %q", cfg.OpenAIApiKey)
	}
	if cfg.AiVisionModel != "gpt-4o-mini" {
		t.Errorf("expected AiVisionModel 'gpt-4o-mini', got %q", cfg.AiVisionModel)
	}
	if cfg.EnvatoApiToken != "test-envato-token" {
		t.Errorf("expected EnvatoApiToken 'test-envato-token', got %q", cfg.EnvatoApiToken)
	}
	if cfg.NanoBananaKey != "test-nano-banana-key" {
		t.Errorf("expected NanoBananaKey 'test-nano-banana-key', got %q", cfg.NanoBananaKey)
	}
	if cfg.VisionSortingPrompt != "Jesteś ekspertem budowlanym..." {
		t.Errorf("expected VisionSortingPrompt match, got %q", cfg.VisionSortingPrompt)
	}
	if cfg.ImageGenerationBasePrompt != "Zdjęcie musi wyglądać jak zrobione amatorsko..." {
		t.Errorf("expected ImageGenerationBasePrompt match, got %q", cfg.ImageGenerationBasePrompt)
	}
}
