package convert

import (
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestConvertDetectsWarnings(t *testing.T) {
	file := excelize.NewFile()
	sheet := file.GetSheetName(0)
	if sheet == "" {
		sheet = "Sheet1"
	}
	file.SetCellValue(sheet, "A1", "Name")
	file.SetCellValue(sheet, "A2", "Asha")
	file.SetCellFormula(sheet, "B2", "SUM(1,2)")
	file.MergeCell(sheet, "A1", "B1")

	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatalf("failed to build xlsx: %v", err)
	}

	res, err := Convert(context.Background(), buffer.Bytes(), Options{MaxSheets: 10, MaxCellsPerSheet: 1000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Sheets) == 0 {
		t.Fatalf("expected sheets to be returned")
	}
	warnings := res.Sheets[0].Warnings
	if len(warnings) == 0 {
		t.Fatalf("expected warnings for merges or formulas")
	}
}
