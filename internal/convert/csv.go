package convert

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func convertCSV(ctx context.Context, input []byte, opts Options, result *Result) error {
	text := bytes.TrimPrefix(input, utf8BOM)
	if !utf8.Valid(text) {
		// Exports from older Excel versions are commonly Windows-1252.
		decoded, err := charmap.Windows1252.NewDecoder().Bytes(text)
		if err == nil {
			text = decoded
		}
	}

	reader := csv.NewReader(bytes.NewReader(text))
	reader.Comma = sniffDelimiter(text)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	rows := [][]string{}
	cells := 0
	for records := 1; ; records++ {
		if records%ctxCheckEvery == 0 {
			if err := checkCtx(ctx); err != nil {
				return err
			}
		}
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.New("this CSV couldn't be parsed: " + err.Error())
		}
		cells += len(record)
		if opts.MaxCellsPerSheet > 0 && cells > opts.MaxCellsPerSheet {
			return ErrSheetTooLarge
		}
		rows = append(rows, trimTrailingEmpty(record))
	}
	if err := checkArea(rows, opts); err != nil {
		return err
	}

	name := strings.TrimSuffix(filepath.Base(opts.Filename), filepath.Ext(opts.Filename))
	if name == "" || name == "." {
		name = "Sheet1"
	}
	result.Meta.SheetCount = 1
	result.Sheets = append(result.Sheets, finishSheet(name, rows, nil))
	return nil
}

// sniffDelimiter picks the separator that appears most consistently in the
// first lines, outside quoted fields.
func sniffDelimiter(text []byte) rune {
	candidates := []rune{',', ';', '\t', '|'}
	lines := strings.SplitN(string(text), "\n", 11)
	if len(lines) > 10 {
		lines = lines[:10]
	}

	best, bestScore := ',', 0
	for _, delim := range candidates {
		counts := []int{}
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			counts = append(counts, countOutsideQuotes(line, delim))
		}
		if len(counts) == 0 || counts[0] == 0 {
			continue
		}
		consistent := 0
		for _, c := range counts {
			if c == counts[0] {
				consistent++
			}
		}
		score := counts[0] * consistent
		if score > bestScore {
			best, bestScore = delim, score
		}
	}
	return best
}

func countOutsideQuotes(line string, delim rune) int {
	count, quoted := 0, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
		case r == delim && !quoted:
			count++
		}
	}
	return count
}
