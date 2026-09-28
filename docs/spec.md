# Excel -> Markdown Conversion Spec (v2)

## Supported Inputs
- File types: `.xlsx` / `.xlsm`, legacy `.xls` (Excel 97-2003, BIFF8), and `.csv` / `.tsv`.
- The format is detected from the file signature first (ZIP = XLSX, OLE = XLS), then the extension (CSV).
- CSV: the delimiter (`,` `;` tab `|`) is sniffed, a UTF-8 BOM is stripped, and non-UTF-8 input is read as Windows-1252. A CSV becomes one sheet named after the file.
- Multi-sheet workbooks are supported. The UI converts several files at once (one request per file).
- Hidden sheets are skipped by default (configurable).

## Sheet Handling Rules
- Each sheet is converted independently and returned in workbook order.
- Each sheet is rendered as a Markdown section header plus a table.
- Empty sheets render a short “no data” note instead of a table.

## Cell Handling Rules
- Text and numeric values are output as plain text.
- Newlines are replaced with `<br>` inside Markdown cells.
- Pipes (`|`) are escaped with `\|`.
- Trailing empty columns are trimmed per row, and table width is the max non-empty column count across rows.
- Rows are padded with empty strings to match the table width.

## Header / Table Structure
- First row is treated as the header row.
- A standard Markdown separator row (`| --- |`) is added after the header.
- If a sheet has only one row, it still becomes the header with an empty body.

## Known Limitations
- Charts, images, pivot tables, and macros are not rendered.
- Merged cells are flattened to the top-left value, and a warning is added.
- XLSX formulas use the result Excel saved in the file. Formulas saved without a result (files written by scripts or libraries) are calculated, best effort: up to 2,000 formulas and 4 s per file, skipping formulas over ranges larger than 100,000 cells. Anything not calculated stays blank and a warning says how many.
- XLSX files that inflate past 256 MB when unzipped are rejected.
- XLS formulas use the result Excel stored in the file (no recalculation).
- Rich text and cell styling are not preserved.

## Limits & Safety
- Max upload size: 50 MB.
- Max sheets: 50.
- Max cells per sheet: 200,000.
- Requests time out if conversion exceeds 10 seconds.

## Environment Variables
- `ADDR`: HTTP bind address (default `:8080`).
- `MAX_UPLOAD_MB`: Max upload size in MB (default `50`).
- `MAX_SHEETS`: Max sheets per workbook (default `50`).
- `MAX_CELLS_PER_SHEET`: Max cells per sheet (default `200000`).
- `CONVERSION_TIMEOUT_SECONDS`: Conversion timeout (default `10`).
- `INCLUDE_HIDDEN_SHEETS`: Include hidden sheets (default `false`).
- `ENABLE_DEBUG_VARS`: Expose `/debug/vars` (default `false`).

## Error & Warning Policy
- If a sheet fails to convert, other sheets still return (partial success).
- Errors are per-sheet when possible, with a top-level failure only for invalid files.
- Warnings include merged cells, formulas, and skipped hidden sheets.

## Docker (local)
- Build image: `docker build -t excel-to-md .`
- Run app only: `docker compose up --build`
- App URL: `http://localhost:8080`
