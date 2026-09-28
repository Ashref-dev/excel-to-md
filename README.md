excel-to-md

Excel to Markdown tables, instantly: https://excel.achraf.tn. Open several .xlsx, .xls or .csv files at once and copy every sheet, or just one, as Markdown.

Features
- Reads .xlsx, legacy .xls and .csv; converts all sheets in each workbook, not just the first
- Uses the formula results Excel saved; formulas saved without a result are calculated (best effort)
- Markdown table output per sheet with combined export
- Drag and drop upload, progress feedback, and copy/download controls
- Handles large workbooks with limits and per-sheet warnings
- Stateless: nothing is stored; files are converted in memory and discarded

Requirements
- Go 1.26+
- Node is not required to build or run

Quick start
- Run: `go run ./cmd/server`
- Open: `http://localhost:8080`
- With portless (named URL, no port clashes; needs Node 24+):
  `portless excel sh -c 'ADDR=127.0.0.1:$PORT go run ./cmd/server'` then open `https://excel.localhost:1355`

Tests
- Run all tests: `go test ./...`

Docker
- Build image
  docker build -t excel-to-md .
- Run app only (default)
  docker compose up --build

Production (https://excel.achraf.tn)
- Self-hosted on a VPS. `deploy/compose.yml` runs the app as `excel-md`, bound to `127.0.0.1:8073` only.
- Public traffic reaches it through a Cloudflare Tunnel whose ingress maps the hostname to `http://127.0.0.1:8073`. Tunnel credentials and config live on the server only, never in this repo.
- Deploy a new version on the server:
  1) `git pull` in the checkout
  2) `docker compose -f deploy/compose.yml up -d --build`
  3) `curl -sI https://excel.achraf.tn` should return 200
- Uploads are limited by the app (MAX_UPLOAD_MB, 50) and by Cloudflare (100 MB per request on Free/Pro).

Configuration
- ADDR (default :8080)
- MAX_UPLOAD_MB (default 50)
- MAX_SHEETS (default 50)
- MAX_CELLS_PER_SHEET (default 200000)
- CONVERSION_TIMEOUT_SECONDS (default 10)
- INCLUDE_HIDDEN_SHEETS (default false)
- ENABLE_DEBUG_VARS (default false)
