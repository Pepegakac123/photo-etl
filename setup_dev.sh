#!/bin/bash
set -e

echo "Setting up local development environment for Photo ETL..."

# 1. Create directories
mkdir -p mock_gallery/Abbrucharbeiten
mkdir -p mock_gallery/Badsanierung_Remont_lazienki
mkdir -p test_client/Abbrucharbeiten
mkdir -p test_client/Fassadenbau
mkdir -p test_client/zrzuty
mkdir -p export

# 2. Create mock photos in gallery and screenshots
# A tiny valid 1x1 PNG pixel
MOCK_IMAGE_BASE64="iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
echo "$MOCK_IMAGE_BASE64" | base64 -d > mock_gallery/Abbrucharbeiten/photo1.jpg
echo "$MOCK_IMAGE_BASE64" | base64 -d > mock_gallery/Badsanierung_Remont_lazienki/photo2.jpg
echo "$MOCK_IMAGE_BASE64" | base64 -d > test_client/zrzuty/screenshot1.jpg
echo "$MOCK_IMAGE_BASE64" | base64 -d > test_client/zrzuty/screenshot2.jpg

# 3. Create a sample config.yaml
cat << EOF > config.yaml
target_photos_per_service: 5
local_gallery_path: "$(pwd)/mock_gallery"
export_dir: "$(pwd)/export"
gopress_cmd_path: ""
concurrency_limit: 5
openai_api_key: ""
ai_vision_model: "gpt-4o-mini"
envato_api_token: ""
nano_banana_key: ""
vision_sorting_prompt: "Jesteś ekspertem budowlanym. Zwróć JSON z przypisaniem zdjęcia do jednej z podanych kategorii. Jeśli zdjęcie to śmieć, zwróć kategorię 'REJECT'."
image_generation_base_prompt: "Zdjęcie musi wyglądać jak zrobione amatorsko, telefonem komórkowym, naturalne oświetlenie na budowie. Złota zasada: brak widocznych twarzy, brak logotypów, brak napisów. Styl surowy i realistyczny."
EOF

echo "--------------------------------------------------"
echo "Done! The local test environment has been prepared."
echo "You can now run the app with:"
echo "  go run cmd/server/main.go -client ./test_client"
echo "--------------------------------------------------"
