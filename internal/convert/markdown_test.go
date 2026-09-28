package convert

import (
	"strings"
	"testing"
)

func TestSheetToMarkdownBasic(t *testing.T) {
	rows := [][]string{
		{"Name", "Age"},
		{"Asha", "29"},
	}
	markdown, _, _ := SheetToMarkdown(rows)
	expected := "| Name | Age |\n| --- | --- |\n| Asha | 29 |"
	if markdown != expected {
		t.Fatalf("unexpected markdown:\n%s", markdown)
	}
}

func TestSheetToMarkdownEmpty(t *testing.T) {
	markdown, _, _ := SheetToMarkdown(nil)
	if markdown != emptySheetMessage {
		t.Fatalf("expected empty sheet message, got %q", markdown)
	}
}

func TestSheetToMarkdownEscaping(t *testing.T) {
	rows := [][]string{
		{"Header"},
		{"A|B"},
		{"Line1\nLine2"},
	}
	markdown, _, _ := SheetToMarkdown(rows)
	if !strings.Contains(markdown, "A\\|B") {
		t.Fatalf("expected pipe to be escaped")
	}
	if !strings.Contains(markdown, "Line1<br>Line2") {
		t.Fatalf("expected newline to be converted to <br>")
	}
}
