package convert

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrTooManySheets     = errors.New("workbook has too many sheets")
	ErrSheetTooLarge     = errors.New("sheet exceeds cell limit")
	ErrConversionTimeout = errors.New("conversion timed out")
	ErrUnsupportedFormat = errors.New("unsupported file: expected .xlsx, .xls or .csv")
	ErrNotAWorkbook      = errors.New("this file isn't a valid Excel workbook; it may be damaged or renamed")
)

// maxAreaFactor bounds the padded table (rows × widest row) relative to the cell limit.
const maxAreaFactor = 4

var (
	zipMagic = []byte("PK\x03\x04")
	oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
)

// Format identifies the kind of spreadsheet in an upload.
type Format string

const (
	FormatXLSX Format = "xlsx"
	FormatXLS  Format = "xls"
	FormatCSV  Format = "csv"
)

// DetectFormat sniffs the file signature first and falls back to the extension,
// so a mislabelled workbook still opens with the right reader.
func DetectFormat(input []byte, filename string) (Format, error) {
	switch {
	case bytes.HasPrefix(input, zipMagic):
		return FormatXLSX, nil
	case bytes.HasPrefix(input, oleMagic):
		return FormatXLS, nil
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".csv", ".tsv", ".txt":
		return FormatCSV, nil
	case ".xlsx", ".xlsm", ".xls":
		return "", ErrNotAWorkbook
	}
	return "", ErrUnsupportedFormat
}

// Convert reads a spreadsheet (XLSX, XLS or CSV) and returns Markdown for each sheet.
func Convert(ctx context.Context, input []byte, opts Options) (Result, error) {
	result := Result{
		Sheets:  []SheetResult{},
		Skipped: []SkippedSheet{},
		Meta:    Meta{GeneratedAt: time.Now().UTC()},
	}

	if err := checkCtx(ctx); err != nil {
		return result, err
	}

	format, err := DetectFormat(input, opts.Filename)
	if err != nil {
		return result, err
	}
	result.Meta.Format = string(format)

	switch format {
	case FormatXLSX:
		err = convertXLSX(ctx, input, opts, &result)
	case FormatXLS:
		err = convertXLS(ctx, input, opts, &result)
	case FormatCSV:
		err = convertCSV(ctx, input, opts, &result)
	}
	if err != nil {
		return result, err
	}

	result.Meta.Processed = len(result.Sheets)
	result.Meta.SkippedCount = len(result.Skipped)
	result.CombinedMarkdown = CombineMarkdown(result.Sheets)
	return result, nil
}

// finishSheet turns extracted rows into a sheet result.
func finishSheet(name string, rows [][]string, warnings []string) SheetResult {
	markdown, rowCount, colCount := SheetToMarkdown(rows)
	return SheetResult{
		Name:     name,
		Markdown: markdown,
		Warnings: warnings,
		RowCount: rowCount,
		ColCount: colCount,
	}
}

// checkArea rejects sheets whose padded Markdown table would dwarf the cell limit,
// e.g. a sparse sheet with one cell in A1 and another thousands of rows and columns away.
func checkArea(rows [][]string, opts Options) error {
	if opts.MaxCellsPerSheet <= 0 {
		return nil
	}
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	if len(rows)*width > opts.MaxCellsPerSheet*maxAreaFactor {
		return ErrSheetTooLarge
	}
	return nil
}

func CombineMarkdown(sheets []SheetResult) string {
	blocks := []string{}
	for _, sheet := range sheets {
		if sheet.Name == "" {
			continue
		}
		blocks = append(blocks, "## "+sheet.Name)
		if sheet.Error != "" {
			blocks = append(blocks, "> Error: "+sheet.Error)
			blocks = append(blocks, "")
			continue
		}
		for _, warning := range sheet.Warnings {
			blocks = append(blocks, "> Note: "+warning)
		}
		if len(sheet.Warnings) > 0 {
			blocks = append(blocks, "")
		}
		blocks = append(blocks, sheet.Markdown)
		blocks = append(blocks, "")
	}
	return strings.Join(blocks, "\n")
}

func checkCtx(ctx context.Context) error {
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ErrConversionTimeout
		}
		return ctx.Err()
	default:
		return nil
	}
}
