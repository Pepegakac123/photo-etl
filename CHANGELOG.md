# Changelog

All notable changes to this project will be documented in this file.

## [1.1.0] - 2026-07-02
### Added
- **Multiple Service Mapping for Client Photos**: Allowed assigning a single client WhatsApp/screenshot photo to multiple services. The client photos view now shows all photos and displays a list of badges under each image indicating which services they are mapped to.
- **Dynamic Initialization Loader Feedback**: Added dynamic text feedback ("Trwa proces tłumaczenia nazw usług na język polski...") to the client loading screen and modal when wczytywanie a client from a non-PL country, providing clear visibility of the translation process.
- **Explicit Clickable Upload Button**: Redesigned manual file upload with a clear, clickable gradient button, fixing DOM/z-index click issues.

### Fixed
- **Optimized Disk Performance**: Removed all sequential file resolution checking from local gallery matches, unmatched client photos, and AI generation outputs, resolving the 15-second disk-read bottleneck and restoring sub-second workspace speed.
- **GoPress Directory Structure**: Restored original GoPress behavior of outputting optimized files into a `webp/` subdirectory and updated gallery-merging code to locate and copy webp files correctly.

## [1.0.9] - 2026-07-02
### Added
- **Unsplash Stock Integration**: Integrated the Unsplash Stock Photos API as a watermark-free alternative for stock image search. Added Unsplash settings fields, connection tests, and workspace tabs to easily search and add high-resolution unwatermarked photos.
- **WhatsApp Folder Export Integration**: Enabled exporting the client's original WhatsApp/screenshots folder as a subdirectory in the final export directory (excluding photos explicitly marked as `rejected` in the database). This ensures that client-supplied raw photo folders are processed, optimized, and uploaded by GoPress instead of being ignored during the final stage.

## [1.0.8] - 2026-07-02
### Added
- **AI Prompt Enhancer**: Added an "Ulepsz prompt z AI (Zróżnicuj)" button next to the prompt customizer. It utilizes `gemini-2.5-flash-lite` via the Google Gemini Interactions API (using the Nano Banana Key) with a temperature of 1.0 to expand Polish service descriptions into diverse, detailed English prompts for highly varied image generation, completely avoiding human faces and full bodies.

### Fixed
- **Minimum Photos Limit on Export**: Changed the target photo count from a hard maximum limit on adding photos to a minimum requirement enforced before starting an export. Users can now add more than the minimum photo limit.
- **Seamless Manual Matching**: Updated the manual photo matching form on the "Zdjęcia Klienta" page to delete the matched card in-place and update both the unmatched badge count and the target service photo counter in the sidebar out-of-band, showing a success toast and avoiding full page reloads.
- **API Resolution Value Correction**: Corrected the option value of the 0.5K resolution selection from `512px` to `512` to comply with the official Google Gemini API parameters.

## [1.0.7] - 2026-07-02
### Fixed
- **Local Gallery Previews Limit Removal**: Removed the arbitrary 6-photo preview limitation when displaying matched/associated folders in the local gallery, showing all available photos within the chosen folder.

## [1.0.6] - 2026-07-02
### Fixed
- **Client Photos Badge Counter Update**: Added an out-of-band (OOB) HTMX update for `#unmatched-count-badge` in `handleWorkspaceUpdate`, ensuring the "Zdjęcia Klienta" sidebar count refreshes instantly during photo actions (such as bulk rejecting pending photos).

## [1.0.5] - 2026-07-02
### Fixed
- **Autocomplete Result Association Race**: Added a 100ms delay to `clearAutocomplete()` on suggestion click to prevent immediate DOM detachment from aborting the HTMX request, fixing the issue where selected folders would fail to load their photo list.

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
