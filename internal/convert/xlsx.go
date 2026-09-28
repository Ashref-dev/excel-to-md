package convert

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

const (
	defaultFormulaBudget = 4 * time.Second
	// Uploads are capped at 50 MB, so a workbook inflating past this is a zip bomb, not a spreadsheet.
	unzipSizeLimit = 256 << 20
	// Formulas are recalculated only when the file carries no saved result. These caps keep a
	// crafted formula from monopolising the request: excelize's calculation can't be interrupted.
	maxFormulaCalcs      = 2000
	maxFormulaRangeCells = 100_000
	maxSheetRows         = 1_048_576
)

var errXLSXDamaged = errors.New("this .xlsx file is damaged or isn't a real Excel workbook")

// rangeRef matches A1:B2 style ranges and whole-column ranges such as A:C.
var rangeRef = regexp.MustCompile(`(?i)\$?([A-Z]{1,3})\$?(\d*)\s*:\s*\$?([A-Z]{1,3})\$?(\d*)`)

type formulaCalc struct {
	file     *excelize.File
	deadline time.Time
	attempts int
	missing  int
}

func convertXLSX(ctx context.Context, input []byte, opts Options, result *Result) error {
	file, err := excelize.OpenReader(bytes.NewReader(input), excelize.Options{UnzipSizeLimit: unzipSizeLimit})
	if err != nil {
		if strings.Contains(err.Error(), "unzip size exceeds") {
			return errors.New("this workbook expands to more data than can be converted")
		}
		return errXLSXDamaged
	}
	defer file.Close()

	sheets := file.GetSheetList()
	result.Meta.SheetCount = len(sheets)
	if opts.MaxSheets > 0 && len(sheets) > opts.MaxSheets {
		return ErrTooManySheets
	}

	budget := opts.FormulaBudget
	if budget <= 0 {
		budget = defaultFormulaBudget
	}
	calc := &formulaCalc{file: file, deadline: time.Now().Add(budget)}

	for _, sheetName := range sheets {
		if err := checkCtx(ctx); err != nil {
			return err
		}

		visible, visErr := file.GetSheetVisible(sheetName)
		if visErr == nil && !visible && !opts.IncludeHiddenSheets {
			result.Skipped = append(result.Skipped, SkippedSheet{Name: sheetName, Reason: "hidden sheet"})
			continue
		}

		rows, warnings, err := extractXLSXSheet(ctx, file, sheetName, calc, opts)
		if err != nil {
			result.Sheets = append(result.Sheets, SheetResult{Name: sheetName, Error: err.Error()})
			continue
		}
		result.Sheets = append(result.Sheets, finishSheet(sheetName, rows, warnings))
	}
	return nil
}

func extractXLSXSheet(ctx context.Context, file *excelize.File, sheetName string, calc *formulaCalc, opts Options) ([][]string, []string, error) {
	warnings := []string{}

	merges, err := file.GetMergeCells(sheetName)
	if err == nil && len(merges) > 0 {
		warnings = append(warnings, "Merged cells were flattened to their top-left value.")
	}

	rows, err := file.Rows(sheetName)
	if err != nil {
		return nil, warnings, errors.New("unable to read this sheet")
	}
	defer rows.Close()

	data := [][]string{}
	rowIndex := 0
	cellCount := 0
	missingBefore := calc.missing

	for rows.Next() {
		if err := checkCtx(ctx); err != nil {
			return nil, warnings, err
		}
		rowIndex++
		cols, err := rows.Columns()
		if err != nil {
			return nil, warnings, errors.New("unable to read this sheet")
		}

		cellCount += len(cols)
		if opts.MaxCellsPerSheet > 0 && cellCount > opts.MaxCellsPerSheet {
			return nil, warnings, ErrSheetTooLarge
		}

		for i, stored := range cols {
			if stored == "" {
				cols[i] = calc.fill(sheetName, i+1, rowIndex)
			}
		}
		data = append(data, trimTrailingEmpty(cols))
	}

	if err := rows.Error(); err != nil {
		return nil, warnings, errors.New("unable to read this sheet")
	}
	if err := checkArea(data, opts); err != nil {
		return nil, warnings, err
	}

	if missed := calc.missing - missingBefore; missed > 0 {
		warnings = append(warnings, fmt.Sprintf("%d formula%s had no saved result and couldn't be calculated.", missed, pluralS(missed)))
	}

	return data, warnings, nil
}

// fill calculates a formula cell that was saved without a result. Cells with a
// saved result keep it: that is Excel's own calculation and the most faithful value.
func (c *formulaCalc) fill(sheet string, col, row int) (out string) {
	defer func() {
		if recover() != nil {
			c.missing++
			out = ""
		}
	}()

	ref, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return ""
	}
	formula, err := c.file.GetCellFormula(sheet, ref)
	if err != nil || formula == "" {
		return ""
	}

	if c.attempts >= maxFormulaCalcs || time.Now().After(c.deadline) || tooWide(formula) {
		c.missing++
		return ""
	}
	c.attempts++

	computed, err := c.file.CalcCellValue(sheet, ref)
	if err != nil {
		c.missing++
		return ""
	}
	return computed
}

func tooWide(formula string) bool {
	for _, m := range rangeRef.FindAllStringSubmatch(formula, -1) {
		c1, err1 := excelize.ColumnNameToNumber(strings.ToUpper(m[1]))
		c2, err2 := excelize.ColumnNameToNumber(strings.ToUpper(m[3]))
		if err1 != nil || err2 != nil {
			continue
		}
		r1, r2 := 1, maxSheetRows
		if m[2] != "" && m[4] != "" {
			r1, _ = strconv.Atoi(m[2])
			r2, _ = strconv.Atoi(m[4])
		}
		cols := abs(c2-c1) + 1
		rows := abs(r2-r1) + 1
		if cols*rows > maxFormulaRangeCells {
			return true
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
