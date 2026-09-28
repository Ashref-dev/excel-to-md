package convert

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/richardlehane/mscfb"
	"github.com/xuri/excelize/v2"
)

// A minimal BIFF8 (Excel 97-2003) reader. It reads cell values, including the
// result Excel cached for each formula, which is the best a legacy file offers.

const (
	recBOF        = 0x0809
	recEOF        = 0x000A
	recBoundSheet = 0x0085
	recSST        = 0x00FC
	recContinue   = 0x003C
	recXF         = 0x00E0
	recFormat     = 0x041E
	recDateMode   = 0x0022
	recFilePass   = 0x002F
	recLabelSST   = 0x00FD
	recLabel      = 0x0204
	recNumber     = 0x0203
	recRK         = 0x027E
	recMulRK      = 0x00BD
	recBoolErr    = 0x0205
	recFormula    = 0x0006
	recString     = 0x0207
	recMergeCells = 0x00E5
)

var (
	errXLSCorrupt        = errors.New("this .xls file is damaged or isn't a real Excel workbook")
	errPasswordProtected = errors.New("this file is password-protected; remove the password and try again")
)

const ctxCheckEvery = 4096

type xlsSheetInfo struct {
	offset uint32
	hidden bool
	kind   byte
	name   string
}

type xlsBook struct {
	stream   []byte
	sheets   []xlsSheetInfo
	sst      []string
	xfFormat []uint16
	formats  map[uint16]string
	date1904 bool
}

func convertXLS(ctx context.Context, input []byte, opts Options, result *Result) (err error) {
	defer func() {
		if recover() != nil {
			err = errXLSCorrupt
		}
	}()

	stream, err := workbookStream(input)
	if err != nil {
		return err
	}
	book, err := parseXLSGlobals(stream)
	if err != nil {
		return err
	}

	worksheets := 0
	for _, info := range book.sheets {
		if info.kind == 0 {
			worksheets++
		}
	}
	result.Meta.SheetCount = worksheets
	if opts.MaxSheets > 0 && worksheets > opts.MaxSheets {
		return ErrTooManySheets
	}

	for _, info := range book.sheets {
		if err := checkCtx(ctx); err != nil {
			return err
		}
		if info.kind != 0 {
			continue // chart, macro and VBA sheets carry no cell data
		}
		if info.hidden && !opts.IncludeHiddenSheets {
			result.Skipped = append(result.Skipped, SkippedSheet{Name: info.name, Reason: "hidden sheet"})
			continue
		}
		rows, warnings, sheetErr := book.readSheet(ctx, info, opts)
		if sheetErr != nil {
			result.Sheets = append(result.Sheets, SheetResult{Name: info.name, Error: sheetErr.Error()})
			continue
		}
		result.Sheets = append(result.Sheets, finishSheet(info.name, rows, warnings))
	}
	return nil
}

func workbookStream(input []byte) ([]byte, error) {
	doc, err := mscfb.New(bytes.NewReader(input))
	if err != nil {
		return nil, errXLSCorrupt
	}
	for entry, nextErr := doc.Next(); nextErr == nil; entry, nextErr = doc.Next() {
		switch entry.Name {
		case "EncryptedPackage", "EncryptionInfo":
			return nil, errPasswordProtected
		case "Workbook":
			if entry.Size <= 0 || entry.Size > int64(len(input)) {
				return nil, errXLSCorrupt
			}
			buf := make([]byte, entry.Size)
			if _, err := io.ReadFull(entry, buf); err != nil {
				return nil, errXLSCorrupt
			}
			return buf, nil
		case "Book":
			return nil, errors.New("this .xls was saved by Excel 95 or older; re-save it as .xlsx and try again")
		}
	}
	return nil, errXLSCorrupt
}

// record iterates BIFF records starting at offset.
type record struct {
	kind uint16
	data []byte
	next int
}

func readRecord(stream []byte, offset int) (record, bool) {
	if offset+4 > len(stream) {
		return record{}, false
	}
	kind := binary.LittleEndian.Uint16(stream[offset:])
	size := int(binary.LittleEndian.Uint16(stream[offset+2:]))
	end := offset + 4 + size
	if end > len(stream) {
		return record{}, false
	}
	return record{kind: kind, data: stream[offset+4 : end], next: end}, true
}

func parseXLSGlobals(stream []byte) (*xlsBook, error) {
	book := &xlsBook{stream: stream, formats: map[uint16]string{}}

	first, ok := readRecord(stream, 0)
	if !ok || first.kind != recBOF || len(first.data) < 2 {
		return nil, errXLSCorrupt
	}
	if binary.LittleEndian.Uint16(first.data) != 0x0600 {
		return nil, errors.New("this .xls was saved by Excel 95 or older; re-save it as .xlsx and try again")
	}

	offset := first.next
	for {
		rec, ok := readRecord(stream, offset)
		if !ok {
			return nil, errXLSCorrupt
		}
		offset = rec.next

		switch rec.kind {
		case recEOF:
			return book, nil
		case recFilePass:
			return nil, errPasswordProtected
		case recDateMode:
			book.date1904 = len(rec.data) >= 2 && binary.LittleEndian.Uint16(rec.data) == 1
		case recXF:
			if len(rec.data) >= 4 {
				book.xfFormat = append(book.xfFormat, binary.LittleEndian.Uint16(rec.data[2:]))
			}
		case recFormat:
			if len(rec.data) >= 5 {
				id := binary.LittleEndian.Uint16(rec.data)
				if text, _, err := readUnicodeString(rec.data[2:], 2); err == nil {
					book.formats[id] = text
				}
			}
		case recBoundSheet:
			if len(rec.data) < 8 {
				return nil, errXLSCorrupt
			}
			name, _, err := readUnicodeString(rec.data[6:], 1)
			if err != nil {
				return nil, errXLSCorrupt
			}
			book.sheets = append(book.sheets, xlsSheetInfo{
				offset: binary.LittleEndian.Uint32(rec.data),
				hidden: rec.data[4]&0x03 != 0,
				kind:   rec.data[5],
				name:   name,
			})
		case recSST:
			chunks := [][]byte{rec.data}
			for {
				cont, ok := readRecord(stream, offset)
				if !ok || cont.kind != recContinue {
					break
				}
				chunks = append(chunks, cont.data)
				offset = cont.next
			}
			sst, err := parseSST(chunks)
			if err != nil {
				return nil, err
			}
			book.sst = sst
		}
	}
}

func (book *xlsBook) readSheet(ctx context.Context, info xlsSheetInfo, opts Options) ([][]string, []string, error) {
	cells := map[int]map[int]string{}
	warnings := []string{}
	merged := false
	count := 0
	set := func(row, col int, value string) {
		if value == "" {
			return
		}
		if cells[row] == nil {
			cells[row] = map[int]string{}
		}
		if _, exists := cells[row][col]; !exists {
			count++
		}
		cells[row][col] = value
	}

	offset := int(info.offset)
	bof, ok := readRecord(book.stream, offset)
	if !ok || bof.kind != recBOF {
		return nil, nil, errors.New("unable to read this sheet")
	}
	offset = bof.next

	for records := 1; ; records++ {
		if records%ctxCheckEvery == 0 {
			if err := checkCtx(ctx); err != nil {
				return nil, nil, err
			}
		}
		if opts.MaxCellsPerSheet > 0 && count > opts.MaxCellsPerSheet {
			return nil, nil, ErrSheetTooLarge
		}

		rec, ok := readRecord(book.stream, offset)
		if !ok {
			return nil, nil, errors.New("this sheet's data ended unexpectedly")
		}
		offset = rec.next
		d := rec.data

		switch rec.kind {
		case recEOF:
			rows, err := denseRows(cells, opts)
			if err != nil {
				return nil, nil, err
			}
			if merged {
				warnings = append(warnings, "Merged cells were flattened to their top-left value.")
			}
			return rows, warnings, nil
		case recMergeCells:
			merged = true
		case recLabelSST:
			if len(d) >= 10 {
				idx := int(binary.LittleEndian.Uint32(d[6:]))
				if idx < len(book.sst) {
					set(u16(d, 0), u16(d, 2), book.sst[idx])
				}
			}
		case recLabel:
			if len(d) >= 9 {
				if text, _, err := readUnicodeString(d[6:], 2); err == nil {
					set(u16(d, 0), u16(d, 2), text)
				}
			}
		case recNumber:
			if len(d) >= 14 {
				value := math.Float64frombits(binary.LittleEndian.Uint64(d[6:]))
				set(u16(d, 0), u16(d, 2), book.formatNumber(value, u16(d, 4)))
			}
		case recRK:
			if len(d) >= 10 {
				set(u16(d, 0), u16(d, 2), book.formatNumber(decodeRK(binary.LittleEndian.Uint32(d[6:])), u16(d, 4)))
			}
		case recMulRK:
			if len(d) >= 6 {
				row, col := u16(d, 0), u16(d, 2)
				for pos := 4; pos+6 <= len(d)-2; pos += 6 {
					value := decodeRK(binary.LittleEndian.Uint32(d[pos+2:]))
					set(row, col, book.formatNumber(value, u16(d, pos)))
					col++
				}
			}
		case recBoolErr:
			if len(d) >= 8 {
				set(u16(d, 0), u16(d, 2), boolErrText(d[6], d[7] == 1))
			}
		case recFormula:
			if len(d) < 14 {
				continue
			}
			row, col, xf := u16(d, 0), u16(d, 2), u16(d, 4)
			res := d[6:14]
			if res[6] != 0xFF || res[7] != 0xFF {
				set(row, col, book.formatNumber(math.Float64frombits(binary.LittleEndian.Uint64(res)), xf))
				continue
			}
			switch res[0] {
			case 0: // string result lives in the following STRING record
				next, ok := readRecord(book.stream, offset)
				if ok && next.kind == recString {
					if text, _, err := readUnicodeString(next.data, 2); err == nil {
						set(row, col, text)
					}
					offset = next.next
				}
			case 1:
				set(row, col, boolErrText(res[2], false))
			case 2:
				set(row, col, boolErrText(res[2], true))
			}
		}
	}
}

func denseRows(cells map[int]map[int]string, opts Options) ([][]string, error) {
	if len(cells) == 0 {
		return [][]string{}, nil
	}
	rowIdx := make([]int, 0, len(cells))
	width := 0
	for r, cols := range cells {
		rowIdx = append(rowIdx, r)
		for c := range cols {
			if c+1 > width {
				width = c + 1
			}
		}
	}
	sort.Ints(rowIdx)
	height := rowIdx[len(rowIdx)-1] + 1
	if opts.MaxCellsPerSheet > 0 && height*width > opts.MaxCellsPerSheet*maxAreaFactor {
		return nil, ErrSheetTooLarge
	}

	rows := make([][]string, height)
	for _, r := range rowIdx {
		maxCol := 0
		for c := range cells[r] {
			if c > maxCol {
				maxCol = c
			}
		}
		row := make([]string, maxCol+1)
		for c, v := range cells[r] {
			row[c] = v
		}
		rows[r] = row
	}
	return rows, nil
}

func u16(d []byte, at int) int {
	return int(binary.LittleEndian.Uint16(d[at:]))
}

func decodeRK(rk uint32) float64 {
	var value float64
	if rk&0x02 != 0 {
		value = float64(int32(rk) >> 2)
	} else {
		value = math.Float64frombits(uint64(rk&0xFFFFFFFC) << 32)
	}
	if rk&0x01 != 0 {
		value /= 100
	}
	return value
}

func boolErrText(value byte, isError bool) string {
	if !isError {
		if value != 0 {
			return "TRUE"
		}
		return "FALSE"
	}
	switch value {
	case 0x00:
		return "#NULL!"
	case 0x07:
		return "#DIV/0!"
	case 0x0F:
		return "#VALUE!"
	case 0x17:
		return "#REF!"
	case 0x1D:
		return "#NAME?"
	case 0x24:
		return "#NUM!"
	case 0x2A:
		return "#N/A"
	}
	return "#ERROR"
}

// formatNumber renders a number the way a reader expects: dates as ISO dates,
// percentages with their sign, everything else at Excel's 15-digit precision.
func (book *xlsBook) formatNumber(value float64, xf int) string {
	var id uint16
	if xf < len(book.xfFormat) {
		id = book.xfFormat[xf]
	}
	code := book.formats[id]

	if isDateFormat(id, code) {
		if t, err := excelize.ExcelDateToTime(value, book.date1904); err == nil {
			var hasDate, hasTime bool
			switch {
			case id >= 18 && id <= 21, id >= 45 && id <= 47:
				hasTime = true
			case id == 22:
				hasDate, hasTime = true, true
			case id < 164:
				hasDate = true
			default:
				lower := strings.ToLower(code)
				hasTime = strings.ContainsAny(lower, "hs")
				hasDate = strings.ContainsAny(lower, "dy") || !hasTime
			}
			switch {
			case hasDate && hasTime:
				return t.Format("2006-01-02 15:04:05")
			case hasTime:
				return t.Format("15:04:05")
			default:
				return t.Format("2006-01-02")
			}
		}
	}

	if id == 9 || id == 10 || strings.Contains(code, "%") {
		return trimFloat(value*100) + "%"
	}
	return trimFloat(value)
}

func trimFloat(value float64) string {
	rounded, err := strconv.ParseFloat(strconv.FormatFloat(value, 'g', 15, 64), 64)
	if err != nil {
		rounded = value
	}
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

func isDateFormat(id uint16, code string) bool {
	switch {
	case id >= 14 && id <= 22, id >= 45 && id <= 47, id >= 27 && id <= 36, id >= 50 && id <= 58:
		return true
	case code == "":
		return false
	}
	// Strip quoted text, escaped characters and [bracketed] sections, then look for date tokens.
	var plain strings.Builder
	inQuote, inBracket, escaped := false, false, false
	for _, r := range strings.ToLower(code) {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case inQuote:
		case r == '[':
			inBracket = true
		case r == ']':
			inBracket = false
		case inBracket:
		default:
			plain.WriteRune(r)
		}
	}
	return strings.ContainsAny(plain.String(), "dmyhs") && !strings.ContainsAny(plain.String(), "#0?")
}

// readUnicodeString reads an XLUnicodeString whose length prefix is lenSize bytes.
func readUnicodeString(d []byte, lenSize int) (string, int, error) {
	if len(d) < lenSize+1 {
		return "", 0, errXLSCorrupt
	}
	count := int(d[0])
	if lenSize == 2 {
		count = int(binary.LittleEndian.Uint16(d))
	}
	flags := d[lenSize]
	pos := lenSize + 1
	if flags&0x01 == 0 {
		if pos+count > len(d) {
			return "", 0, errXLSCorrupt
		}
		return latin1(d[pos : pos+count]), pos + count, nil
	}
	if pos+2*count > len(d) {
		return "", 0, errXLSCorrupt
	}
	return utf16le(d[pos : pos+2*count]), pos + 2*count, nil
}

func latin1(b []byte) string {
	runes := make([]rune, len(b))
	for i, c := range b {
		runes[i] = rune(c)
	}
	return string(runes)
}

func utf16le(b []byte) string {
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(units))
}

// sstReader walks the SST record and its CONTINUE records as one stream,
// honouring the rule that a string split across records restates its flags.
type sstReader struct {
	chunks [][]byte
	chunk  int
	pos    int
}

func (r *sstReader) ensure() error {
	for r.chunk < len(r.chunks) && r.pos >= len(r.chunks[r.chunk]) {
		r.chunk++
		r.pos = 0
	}
	if r.chunk >= len(r.chunks) {
		return errXLSCorrupt
	}
	return nil
}

func (r *sstReader) byte() (byte, error) {
	if err := r.ensure(); err != nil {
		return 0, err
	}
	b := r.chunks[r.chunk][r.pos]
	r.pos++
	return b, nil
}

func (r *sstReader) uint(size int) (uint32, error) {
	var value uint32
	for i := 0; i < size; i++ {
		b, err := r.byte()
		if err != nil {
			return 0, err
		}
		value |= uint32(b) << (8 * i)
	}
	return value, nil
}

func (r *sstReader) skip(n int) error {
	for n > 0 {
		if err := r.ensure(); err != nil {
			return err
		}
		step := len(r.chunks[r.chunk]) - r.pos
		if step > n {
			step = n
		}
		r.pos += step
		n -= step
	}
	return nil
}

func (r *sstReader) chars(count int, wide bool) (string, error) {
	units := make([]uint16, 0, count)
	for len(units) < count {
		if r.pos >= len(r.chunks[r.chunk]) {
			// Characters continue in the next CONTINUE record, which restates the width flag.
			r.chunk++
			r.pos = 0
			if r.chunk >= len(r.chunks) || len(r.chunks[r.chunk]) == 0 {
				return "", errXLSCorrupt
			}
			wide = r.chunks[r.chunk][0]&0x01 != 0
			r.pos = 1
			continue
		}
		data := r.chunks[r.chunk]
		if wide {
			if r.pos+2 > len(data) {
				return "", errXLSCorrupt
			}
			units = append(units, binary.LittleEndian.Uint16(data[r.pos:]))
			r.pos += 2
		} else {
			units = append(units, uint16(data[r.pos]))
			r.pos++
		}
	}
	return string(utf16.Decode(units)), nil
}

func parseSST(chunks [][]byte) ([]string, error) {
	r := &sstReader{chunks: chunks}
	if _, err := r.uint(4); err != nil { // total references
		return nil, err
	}
	unique, err := r.uint(4)
	if err != nil {
		return nil, err
	}
	// Every string takes at least 3 bytes (length + flags), so a count beyond that is a lie.
	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}
	if int(unique) > total/3 {
		return nil, errXLSCorrupt
	}

	out := make([]string, 0, unique)
	for i := 0; i < int(unique); i++ {
		count, err := r.uint(2)
		if err != nil {
			return out, nil // tolerate a truncated table; missing strings stay blank
		}
		flags, err := r.byte()
		if err != nil {
			return out, nil
		}
		runs, ext := 0, 0
		if flags&0x08 != 0 {
			v, _ := r.uint(2)
			runs = int(v)
		}
		if flags&0x04 != 0 {
			v, _ := r.uint(4)
			ext = int(v)
		}
		text, err := r.chars(int(count), flags&0x01 != 0)
		if err != nil {
			return out, nil
		}
		out = append(out, text)
		if err := r.skip(4*runs + ext); err != nil && i < int(unique)-1 {
			return out, nil
		}
	}
	return out, nil
}
