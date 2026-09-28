package convert

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func convertOrFail(t *testing.T, data []byte, name string) Result {
	t.Helper()
	res, err := Convert(context.Background(), data, Options{Filename: name, MaxSheets: 10, MaxCellsPerSheet: 1000})
	if err != nil {
		t.Fatalf("convert %s: %v", name, err)
	}
	return res
}

func TestXLSXFormulasAreCalculated(t *testing.T) {
	file := excelize.NewFile()
	file.SetCellValue("Sheet1", "A1", "a")
	file.SetCellValue("Sheet1", "B1", "b")
	file.SetCellValue("Sheet1", "C1", "sum")
	file.SetCellValue("Sheet1", "A2", 2)
	file.SetCellValue("Sheet1", "B2", 3)
	file.SetCellFormula("Sheet1", "C2", "A2*B2+1") // no cached value stored
	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	res := convertOrFail(t, buffer.Bytes(), "calc.xlsx")
	if got := res.Sheets[0].Markdown; !strings.Contains(got, "| 2 | 3 | 7 |") {
		t.Fatalf("formula not calculated:\n%s", got)
	}
	if len(res.Sheets[0].Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Sheets[0].Warnings)
	}
}

func TestXLSXWideRangeFormulaIsNotEvaluated(t *testing.T) {
	file := excelize.NewFile()
	file.SetCellValue("Sheet1", "A1", "total")
	file.SetCellFormula("Sheet1", "A2", "SUM(B:D)")
	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	res := convertOrFail(t, buffer.Bytes(), "wide.xlsx")
	warnings := res.Sheets[0].Warnings
	if len(warnings) != 1 || !strings.Contains(warnings[0], "1 formula had no saved result") {
		t.Fatalf("expected a fallback warning, got %v", warnings)
	}
}

func TestPasswordProtectedXLSX(t *testing.T) {
	file := excelize.NewFile()
	file.SetCellValue("Sheet1", "A1", "secret")
	var buffer bytes.Buffer
	if err := file.Write(&buffer, excelize.Options{Password: "pw"}); err != nil {
		t.Fatal(err)
	}

	_, err := Convert(context.Background(), buffer.Bytes(), Options{Filename: "locked.xlsx"})
	if !errors.Is(err, errPasswordProtected) {
		t.Fatalf("expected password error, got %v", err)
	}
}

func TestLegacyXLS(t *testing.T) {
	data, err := os.ReadFile("testdata/legacy.xls")
	if err != nil {
		t.Fatal(err)
	}
	res := convertOrFail(t, data, "legacy.xls")
	if res.Meta.Format != "xls" {
		t.Fatalf("format = %q", res.Meta.Format)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Name != "Caché" {
		t.Fatalf("hidden sheet not skipped: %+v", res.Skipped)
	}
	md := res.Sheets[0].Markdown
	for _, want := range []string{"| Région | Montant | Date | Part |", "| Tunis | 1250.5 | 2026-03-14 | 12.5% |", `Sfax \| Sud`, "日本語"} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in:\n%s", want, md)
		}
	}
}

func TestCSVDelimiterAndEncoding(t *testing.T) {
	res := convertOrFail(t, []byte("name;score\n\"Ben Salem, Amira\";9,5\n"), "scores.csv")
	if res.Sheets[0].Name != "scores" {
		t.Fatalf("sheet name = %q", res.Sheets[0].Name)
	}
	if !strings.Contains(res.Sheets[0].Markdown, "| Ben Salem, Amira | 9,5 |") {
		t.Fatalf("semicolon CSV misparsed:\n%s", res.Sheets[0].Markdown)
	}

	latin := convertOrFail(t, []byte("nom\nFran\xe7ois\n"), "latin.csv")
	if !strings.Contains(latin.Sheets[0].Markdown, "François") {
		t.Fatalf("windows-1252 not decoded:\n%s", latin.Sheets[0].Markdown)
	}
}

func TestDetectFormat(t *testing.T) {
	if _, err := DetectFormat([]byte("%PDF-1.7"), "report.pdf"); err == nil {
		t.Fatal("expected unsupported format")
	}
	if f, _ := DetectFormat([]byte("PK\x03\x04rest"), "misnamed.xls"); f != FormatXLSX {
		t.Fatalf("zip signature should win over extension, got %q", f)
	}
	if _, err := DetectFormat([]byte("not a workbook"), "report.xlsx"); !errors.Is(err, ErrNotAWorkbook) {
		t.Fatalf("expected ErrNotAWorkbook for a fake .xlsx, got %v", err)
	}
}
