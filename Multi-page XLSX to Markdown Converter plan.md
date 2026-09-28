# Interpreted Feature Summary
You want a modern, elegant, UX-focused converter that takes a multi-sheet Excel (.xlsx) file and outputs clean, organized Markdown for every sheet (not just the first), with easy copy/paste and download. The tool should handle errors and edge cases gracefully, and you prefer a Go (Golang) backend with a security-conscious approach. Primary visual direction includes a main accent color of #A688F1.

# Assumptions & Unknowns
Assumptions:
- The initial product is a web app with a single upload flow and a results page that aggregates per-sheet Markdown output.
- Input files are .xlsx only (not .xls, .csv, or Google Sheets links) for v1.
- Output is standard GitHub-Flavored Markdown tables for each sheet.
- Each sheet is converted independently and placed under a clear Markdown section header.
- File size limits and conversion timeouts are required to prevent abuse.
- Go is used for the backend conversion service; a lightweight web frontend (any modern JS framework or server-rendered pages) can be used.

Unknowns / open design choices:
- Maximum allowed file size and number of sheets.
- How to handle complex Excel features (merged cells, formulas, images, charts, pivot tables, hidden sheets).
- Whether to preserve cell formatting (bold/italic, colors) or strip to plain text.
- Whether to allow per-sheet export/download or only a single combined Markdown file.
- Preferred deployment target (self-hosted, serverless, containerized).

# Success Criteria
## User Success
- User can upload a multi-sheet .xlsx and get Markdown for all sheets in one organized view.
- Markdown is clean, readable, and easy to copy or download.
- Errors are understandable and actionable (e.g., “File too large”, “Unsupported format”).
- Processing completes quickly for typical files (seconds, not minutes).

## System / Business Success
- >99% of uploads convert successfully without manual intervention.
- Median conversion time under 2 seconds for files under the size limit.
- Low error rate for supported .xlsx inputs.
- Clear UX reduces abandonment vs. current “first sheet only” tools.

## Non-Goals
- Full fidelity Excel rendering (charts, images, pivot tables, macros).
- Supporting .xls or Google Sheets URLs in v1.
- Round-trip conversion (Markdown back to Excel).
- Advanced styling or custom Markdown themes per sheet in v1.

# UX Plan
## Primary Flow
1. Landing: brief value prop, upload control (drag & drop + browse), and size/format hints.
2. Upload: file is validated client-side (extension, size) then sent to backend.
3. Processing: progress indicator; if fast, show a short “Converting sheets…” status.
4. Results: per-sheet sections with sheet name headers and rendered Markdown table previews.
5. Actions: “Copy all”, “Copy sheet”, and “Download .md” (combined or per sheet).

## UI States & Edge Cases
- Empty state: no file uploaded yet.
- Loading state: conversion in progress; show sheet count if available.
- Error state: invalid format, too large, corrupted file, conversion failure.
- Edge cases: empty sheets, hidden sheets, merged cells, formulas, very large sheets.
- Partial success: if a sheet fails, show it as failed but still return others.

## Accessibility
- Keyboard-friendly upload and actions.
- Clear focus states and ARIA labels for upload and copy buttons.
- Sufficient contrast for primary color (#A688F1) and text.
- Screen-reader readable sheet names and error messages.

# Technical Strategy (Conceptual)
- System boundaries:
  - Frontend: handles upload UI, progress, and results rendering.
  - Backend (Go): parses .xlsx, converts sheets to Markdown, returns structured result.
- Core components:
  - XLSX parser library (Go) to read sheets, cells, and metadata.
  - Markdown generator: takes 2D cell data and creates a Markdown table.
  - API endpoint: accepts multipart file upload, returns JSON with per-sheet Markdown or errors.
- Data:
  - Input: file bytes, sheet names, cell values.
  - Output: array of sheets {name, markdown, warnings}.
- API interactions:
  - POST /convert: returns structured results; supports partial success.
- Security/privacy:
  - Validate file size and extension.
  - Timeouts and memory limits per request.
  - No file persistence unless explicitly enabled.
- Performance:
  - Stream or chunk parsing for large files if supported by library.
  - Early exit on file limit or malformed data.
  - Cache is not required for v1.

# Risks & Tradeoffs
- Product risk: users expect full Excel fidelity; mitigation: clear scope messaging.
- UX risk: very large sheets produce huge Markdown; mitigation: collapsible previews and download.
- Technical risk: merged cells and formulas may render poorly; mitigation: warnings and fallback to plain values.
- Security risk: malicious files; mitigation: strict validation, sandboxed processing, timeouts.
- Tradeoff: speed and simplicity over perfect layout fidelity.

# Step-by-Step Implementation Plan, with numbered sections and tasks and code snippets if needed.
## Milestone 1
Objective: Define format scope and conversion rules.
Tasks:
- [x] Document supported Excel features and known limitations.
- [x] Decide how to handle merged cells, empty cells, hidden sheets, and formulas.
- [x] Define Markdown output structure: section header per sheet, table format, and optional warnings.
Files/components likely involved:
- docs/spec.md (new)
Validation steps:
- Review spec for clarity and completeness.
Tests to add:
- None (documentation milestone).
Dependencies:
- None.

## Milestone 2
Objective: Build backend conversion service (Go) with a stable API.
Tasks:
- [x] Select Go XLSX library and evaluate handling of multi-sheet data.
- [x] Implement /convert endpoint with multipart upload.
- [x] Convert each sheet to Markdown with consistent table formatting.
- [x] Add error handling for invalid files and partial sheet failures.
- [x] Add request size and time limits.
Files/components likely involved:
- cmd/server
- internal/convert
- internal/http
Validation steps:
- Local API test with a known multi-sheet file.
- Confirm JSON output includes all sheets.
Tests to add:
- Unit tests for Markdown generation (empty sheets, wide sheets).
- Unit tests for error handling (invalid file, too large).
Dependencies:
- Milestone 1 spec.
Feature flags/staging:
- Optional: flag for “include hidden sheets”.

## Milestone 3
Objective: Build frontend UI for upload and results.
Tasks:
- [x] Create upload UI with drag-and-drop and file validation.
- [x] Implement progress/loading state.
- [x] Render per-sheet Markdown sections with copy and download actions.
- [x] Add error and partial success UI states.
- [x] Apply visual design with #A688F1 accent and modern typography.
Files/components likely involved:
- web/ (or equivalent frontend folder)
- UI components for Upload, ResultList, SheetCard
Validation steps:
- Manual flow test: upload, convert, copy, download.
Tests to add:
- Component tests for error and empty states.
Dependencies:
- Milestone 2 API.

## Milestone 4
Objective: Harden for production and edge cases.
Tasks:
- [x] Add server-side limits (size, sheet count, cell count).
- [x] Implement warnings for unsupported features (merged cells, formulas).
- [x] Add observability: logs and basic metrics.
- [x] Add export options (combined Markdown vs per-sheet).
Files/components likely involved:
- internal/convert
- internal/http
- web/ export actions
Validation steps:
- Stress test with large files within limits.
Tests to add:
- End-to-end test: multi-sheet with edge cases.
Dependencies:
- Milestone 2 and 3.

## Milestone 5
Objective: Production readiness and security verification.
Tasks:
- [x] Add security headers and tighten static file handling.
- [x] Add database integration (optional) via PostgreSQL connection string.
- [x] Update configuration for DB and operational settings.
- [x] Run build/test/vet checks and fix any issues.
Files/components likely involved:
- internal/server
- internal/config
- internal/storage (new)
Validation steps:
- go test ./...
- go vet ./...
- go build ./cmd/server
Tests to add:
- Optional: storage unit tests (if feasible).
Dependencies:
- Milestone 2-4 complete.

## Milestone 6
Objective: Dockerize for production deployment.
Tasks:
- [x] Add Dockerfile for multi-stage build.
- [x] Add docker-compose for local Postgres + app.
- [x] Document environment variables and run instructions.
Files/components likely involved:
- Dockerfile
- docker-compose.yml
- docs/spec.md (env updates)
Validation steps:
- docker build
- docker compose up (optional)
Dependencies:
- Milestone 5 complete.

# Test & Validation Plan
- Unit tests: Markdown table rendering rules, edge cases, error mapping.
- Integration tests: /convert endpoint with multi-sheet inputs and partial failures.
- UI tests: upload flow, loading state, error display, copy/download actions.
- Manual QA: verify output readability and sheet order matches Excel.

# Handoff Pack for Implementation Agents
Ordered task checklist:
- [x] Confirm scope and document conversion rules (Milestone 1).
- [x] Implement Go conversion service and /convert API (Milestone 2).
- [x] Build frontend upload + results UI (Milestone 3).
- [x] Add limits, warnings, and export options (Milestone 4).
- [x] Production readiness + security verification (Milestone 5).
- [x] Dockerize for deployment (Milestone 6).

Assumptions they must respect:
- v1 only supports .xlsx; multi-sheet output is mandatory.
- Use simple Markdown tables with sheet headers.
- No charts/images/pivots in v1.
- Security limits (file size, timeouts) are required.

Definition of Done:
- Multi-sheet .xlsx converts into organized Markdown for all sheets.
- UI supports upload, progress, errors, and copy/download actions.
- Clear messaging for unsupported features and partial failures.
- Tests cover core conversion logic and error states.

What NOT to change without revisiting the plan:
- Switching away from Go backend.
- Expanding scope to other formats or full fidelity Excel rendering.
- Removing security limits or error handling.
