package convert

import (
	"strings"
)

const emptySheetMessage = "_No data in this sheet._"

// SheetToMarkdown converts a 2D slice of strings into a Markdown table.
func SheetToMarkdown(rows [][]string) (string, int, int) {
	if len(rows) == 0 {
		return emptySheetMessage, 0, 0
	}

	trimmedRows := make([][]string, 0, len(rows))
	maxCols := 0
	for _, row := range rows {
		trimmed := trimTrailingEmpty(row)
		if len(trimmed) > maxCols {
			maxCols = len(trimmed)
		}
		trimmedRows = append(trimmedRows, trimmed)
	}

	trimmedRows = trimTrailingEmptyRows(trimmedRows)

	if len(trimmedRows) == 0 || maxCols == 0 {
		return emptySheetMessage, 0, 0
	}

	normalized := padRows(trimmedRows, maxCols)

	lines := make([]string, 0, len(normalized)+1)
	lines = append(lines, formatRow(normalized[0]))
	lines = append(lines, formatSeparator(maxCols))

	for i := 1; i < len(normalized); i++ {
		lines = append(lines, formatRow(normalized[i]))
	}

	return strings.Join(lines, "\n"), len(normalized), maxCols
}

func trimTrailingEmpty(row []string) []string {
	end := len(row)
	for end > 0 {
		value := row[end-1]
		if strings.TrimSpace(value) != "" {
			break
		}
		end--
	}
	if end == 0 {
		return []string{}
	}
	trimmed := make([]string, end)
	copy(trimmed, row[:end])
	return trimmed
}

func trimTrailingEmptyRows(rows [][]string) [][]string {
	end := len(rows)
	for end > 0 {
		if len(rows[end-1]) > 0 {
			break
		}
		end--
	}
	if end == 0 {
		return [][]string{}
	}
	return rows[:end]
}

func padRows(rows [][]string, width int) [][]string {
	padded := make([][]string, len(rows))
	for i, row := range rows {
		rowCopy := make([]string, width)
		copy(rowCopy, row)
		padded[i] = rowCopy
	}
	return padded
}

func formatRow(row []string) string {
	parts := make([]string, len(row))
	for i, cell := range row {
		parts[i] = escapeCell(cell)
	}
	return "| " + strings.Join(parts, " | ") + " |"
}

func formatSeparator(width int) string {
	parts := make([]string, width)
	for i := range parts {
		parts[i] = "---"
	}
	return "| " + strings.Join(parts, " | ") + " |"
}

func escapeCell(value string) string {
	if value == "" {
		return ""
	}
	escaped := strings.ReplaceAll(value, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	escaped = strings.ReplaceAll(escaped, "|", "\\|")
	return escaped
}
