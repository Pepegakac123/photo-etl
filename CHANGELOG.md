# Changelog

All notable changes to this project will be documented in this file.

## [1.0.4] - 2026-07-02
### Added
- **AI Image Generation Resolution Options**: Added a dropdown select in the AI generation tab to choose between 512px (0.5K), 1K, 2K, and 4K output sizes.
- **Cheap Default Settings**: Swapped the default model to `gemini-3.1-flash-lite-image` (Nano Banana 2 Lite) and default resolution to `1K` to significantly reduce API costs for previews.

## [1.0.3] - 2026-07-02
### Added
- **Scroll Position Preservation**: Implemented a client-side JavaScript scroll preserver that saves and restores the scroll location of the project photos list during HTMX swaps.
- **Reject All Remaining Pending Photos**: Added a quick action button in the project photos sidebar that rejects all remaining pending images for the current service with a single click.

## [1.0.2] - 2026-07-02
### Added
- **Manual Disk File Upload**: Added a visual upload dropzone in the project photos sidebar, allowing users to upload and assign multiple photos directly from their disk to any service.
- **Manual Gallery Reindexing**: Added an "Update Index" button in the local gallery tab to re-scan and index newly added folders and images on the fly.

## [1.0.1] - 2026-07-02
### Added
- **Dual AI Classification Mode**: Added separate buttons for fast testing (limit 5 unprocessed images) and full sorting (unlimited, all unprocessed images).
- **Unprocessed Image Filtering**: AI Vision classification now dynamically queries the database and skips files that have already been classified, fixing the test limit counter issue.

## [1.0.0] - 2026-07-02
### Added
- **Cross-Platform Release Workflow**: Added GitHub Actions workflow to build and release static compiled binaries for Linux, Windows (AMD64), and macOS (AMD64 & ARM64).
- **Auto-Update Mechanism**: Integrated GitHub self-updating directly into the CLI tool (`-update` flag) with dynamic release log printing.
- **Dynamic Autocomplete Folder Search**: Added keystroke-based autocomplete search for local gallery folders inside the workspace.
- **Skeleton Loader States**: Implemented animated loading placeholder skeletons during AI Nano Banana image generation.
- **Prompt and Context Customizer**: Added a customizable textarea to preview, edit, and persist the service context description before generation.
- **GoPress Integration**: Optimized pictures using GoPress CLI and implemented real-time SSE progress console output.
- **WhatsApp Screenshot Filtering**: Added unmatched/rejected client screenshot workspace and visual mapper.
- **Central Gallery Exclusion**: Prevented client WhatsApp screenshots from being merged into the reusable central local gallery during export/GoPress merging.
- **Envato Stock Search**: Integrated Envato Stock photo search with automatic resolution enhancement up to 1920px width.
