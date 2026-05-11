package markdowntopdf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
)

// FontFamily is the logical font we register under a single PDF family name.
// We intentionally use one face for all content (headings, body, code,
// tables) to keep complexity low and avoid bundling multiple system fonts.
const FontFamily = "default"

// sfnt magic numbers.
const (
	sfntMagicTrueType uint32 = 0x00010000 // TrueType outlines
	sfntMagicCFF      uint32 = 0x4F54544F // "OTTO" — PostScript/CFF outlines (signintech/gopdf does NOT support)
	ttcMagic          uint32 = 0x74746366 // "ttcf"
)

// ErrFontNotFound is returned when probing cannot find any usable CJK font.
var ErrFontNotFound = errors.New("no system CJK font found")

// Font is a resolved TTF font ready to be handed to gopdf.
type Font struct {
	// Data is the raw TTF byte stream (TrueType outlines only).
	Data []byte
	// SourcePath records where the font came from (for logging/diagnostics).
	SourcePath string
	// FontIndex is the TTC index that was extracted (0 if the source was already TTF).
	FontIndex int
}

// ResolveCJKFont searches for a usable Chinese-capable TrueType font, probing
// in this order:
//  1. explicitPath (if non-empty) — typically from tool arguments
//  2. ARK_MD_PDF_FONT environment variable
//  3. OS-specific default paths (macOS → Linux → Windows)
//
// If the resolved file is a TrueType Collection (.ttc), the first font at
// index 0 is extracted into a standalone TTF byte stream. OpenType/CFF
// outlines (OTTO / .otf) are rejected because the underlying gopdf TTF
// parser only supports TrueType (sfnt magic 0x00010000).
func ResolveCJKFont(explicitPath string) (*Font, error) {
	candidates := make([]string, 0, 8)
	if p := strings.TrimSpace(explicitPath); p != "" {
		candidates = append(candidates, p)
	}
	if p := strings.TrimSpace(os.Getenv("ARK_MD_PDF_FONT")); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, defaultFontSearchPaths()...)

	var lastErr error
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}
		ttf, idx, err := ensureTrueTypeTTF(data, 0)
		if err != nil {
			lastErr = fmt.Errorf("parse %s: %w", path, err)
			continue
		}
		return &Font{Data: ttf, SourcePath: path, FontIndex: idx}, nil
	}

	hint := fontInstallHint()
	if lastErr != nil {
		return nil, fmt.Errorf("%w (last error: %v); %s", ErrFontNotFound, lastErr, hint)
	}
	return nil, fmt.Errorf("%w; %s", ErrFontNotFound, hint)
}

// defaultFontSearchPaths returns OS-specific font locations likely to contain
// a CJK-capable TrueType font. Ordered by preference within each OS.
func defaultFontSearchPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			// Songti SC is the canonical macOS 宋体 — best match for the user's
			// "system 宋体" request.
			"/System/Library/Fonts/Supplemental/Songti.ttc",
			"/Library/Fonts/Songti.ttc",
			// PingFang SC — modern sans-serif, ships on all recent macOS.
			"/System/Library/Fonts/PingFang.ttc",
			// STHeiti — older but always present as a fallback.
			"/System/Library/Fonts/STHeiti Light.ttc",
			"/System/Library/Fonts/STHeiti Medium.ttc",
			// Hiragino — shipped with macOS, covers GB.
			"/System/Library/Fonts/Supplemental/Hiragino Sans GB.ttc",
			"/System/Library/Fonts/Hiragino Sans GB.ttc",
		}
	case "linux":
		return []string{
			// WenQuanYi Zen Hei — small (~10 MB), universally packaged as
			// fonts-wqy-zenhei in Debian/Ubuntu, recommended for headless
			// containers.
			"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
			"/usr/share/fonts/wqy-zenhei/wqy-zenhei.ttc",
			"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
			"/usr/share/fonts/wqy-microhei/wqy-microhei.ttc",
			// Noto CJK — higher quality but larger (install via fonts-noto-cjk).
			"/usr/share/fonts/opentype/noto/NotoSerifCJK-Regular.ttc",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/noto/NotoSerifCJK-Regular.ttc",
			"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/noto-cjk/NotoSerifCJK-Regular.ttc",
			"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
			// ARPHIC — older but small (fonts-arphic-uming / ukai).
			"/usr/share/fonts/truetype/arphic/uming.ttc",
			"/usr/share/fonts/truetype/arphic/ukai.ttc",
		}
	case "windows":
		// Backslash paths are fine for os.ReadFile on Windows.
		return []string{
			`C:\Windows\Fonts\simsun.ttc`,
			`C:\Windows\Fonts\simfang.ttf`,
			`C:\Windows\Fonts\simhei.ttf`,
			`C:\Windows\Fonts\simkai.ttf`,
			`C:\Windows\Fonts\msyh.ttc`,
			`C:\Windows\Fonts\msyhbd.ttc`,
		}
	}
	return nil
}

// fontInstallHint returns a human-readable suggestion for how to make a
// suitable font available on the current OS.
func fontInstallHint() string {
	switch runtime.GOOS {
	case "linux":
		return "set ARK_MD_PDF_FONT to a TTF path or install a CJK font (e.g. `apt-get install fonts-wqy-zenhei` or `apt-get install fonts-noto-cjk`)"
	case "windows":
		return "set ARK_MD_PDF_FONT to a TTF path (e.g. C:\\Windows\\Fonts\\simsun.ttc)"
	case "darwin":
		return "set ARK_MD_PDF_FONT to a TTF path; macOS normally ships Songti.ttc under /System/Library/Fonts/Supplemental/"
	}
	return "set ARK_MD_PDF_FONT to a TTF path containing CJK glyphs"
}

// ensureTrueTypeTTF inspects the input bytes and returns a standalone
// TrueType-only TTF byte stream plus the selected font index.
//
//   - If the input already looks like a TrueType file (magic 0x00010000),
//     it is returned unchanged with index 0.
//   - If the input is a TTC (magic 'ttcf'), the font at fontIndex (clamped
//     to the available range) is extracted and re-packed as a standalone
//     TTF.
//   - OTF / CFF-flavored fonts (magic 'OTTO') are rejected because
//     signintech/gopdf's parser requires TrueType outlines.
func ensureTrueTypeTTF(data []byte, fontIndex int) ([]byte, int, error) {
	if len(data) < 12 {
		return nil, 0, fmt.Errorf("font file too small: %d bytes", len(data))
	}
	magic := binary.BigEndian.Uint32(data[0:4])
	switch magic {
	case sfntMagicTrueType:
		return data, 0, nil
	case sfntMagicCFF:
		return nil, 0, errors.New("OpenType/CFF fonts (.otf) are not supported; need TrueType outlines")
	case ttcMagic:
		return extractTTFFromTTC(data, fontIndex)
	default:
		return nil, 0, fmt.Errorf("unrecognized font magic: 0x%08x", magic)
	}
}

// extractTTFFromTTC parses a TrueType Collection and re-packs the font at
// the given index into a standalone TTF byte stream. The selected sub-font
// must use TrueType outlines (CFF sub-fonts are rejected).
//
// TTC format reference: https://learn.microsoft.com/en-us/typography/opentype/spec/otff#font-collections
func extractTTFFromTTC(data []byte, requestedIndex int) ([]byte, int, error) {
	if len(data) < 12 {
		return nil, 0, errors.New("TTC header truncated")
	}
	// TTC header layout:
	//   uint32 ttcTag ('ttcf')
	//   uint16 majorVersion
	//   uint16 minorVersion
	//   uint32 numFonts
	//   uint32 offsets[numFonts]
	numFonts := int(binary.BigEndian.Uint32(data[8:12]))
	if numFonts == 0 {
		return nil, 0, errors.New("TTC contains zero fonts")
	}
	if 12+4*numFonts > len(data) {
		return nil, 0, errors.New("TTC offset table truncated")
	}
	idx := requestedIndex
	if idx < 0 || idx >= numFonts {
		idx = 0
	}

	// Try each sub-font starting from the requested one; some TTCs
	// interleave TrueType and CFF sub-fonts, and we can only use the former.
	for attempt := 0; attempt < numFonts; attempt++ {
		tryIdx := (idx + attempt) % numFonts
		fontOffset := int(binary.BigEndian.Uint32(data[12+4*tryIdx : 16+4*tryIdx]))
		ttf, err := rebuildTTFFromOffsetTable(data, fontOffset)
		if err != nil {
			continue
		}
		return ttf, tryIdx, nil
	}
	return nil, 0, errors.New("no TrueType sub-font found in TTC")
}

// rebuildTTFFromOffsetTable reads the sfnt Offset Table at offset
// `tableStart` inside a TTC file and rebuilds a standalone TTF by copying
// all referenced tables into a fresh container with relocated offsets.
//
// Offset Table layout:
//
//	uint32 sfntVersion   (must be 0x00010000 for TrueType)
//	uint16 numTables
//	uint16 searchRange, entrySelector, rangeShift
//	TableRecord records[numTables]
//
// TableRecord (16 bytes):
//
//	uint32 tag
//	uint32 checkSum
//	uint32 offset      — absolute file offset to the table data
//	uint32 length
func rebuildTTFFromOffsetTable(data []byte, tableStart int) ([]byte, error) {
	if tableStart < 0 || tableStart+12 > len(data) {
		return nil, errors.New("offset table truncated")
	}
	sfntVersion := binary.BigEndian.Uint32(data[tableStart : tableStart+4])
	if sfntVersion != sfntMagicTrueType {
		return nil, fmt.Errorf("sub-font is not TrueType (sfnt=0x%08x)", sfntVersion)
	}
	numTables := int(binary.BigEndian.Uint16(data[tableStart+4 : tableStart+6]))
	if numTables == 0 || numTables > 256 {
		return nil, fmt.Errorf("unreasonable numTables: %d", numTables)
	}
	recordsStart := tableStart + 12
	if recordsStart+16*numTables > len(data) {
		return nil, errors.New("table records truncated")
	}

	type record struct {
		tag      [4]byte
		checkSum uint32
		origOff  uint32
		length   uint32
	}

	records := make([]record, numTables)
	for i := range records {
		base := recordsStart + 16*i
		copy(records[i].tag[:], data[base:base+4])
		records[i].checkSum = binary.BigEndian.Uint32(data[base+4 : base+8])
		records[i].origOff = binary.BigEndian.Uint32(data[base+8 : base+12])
		records[i].length = binary.BigEndian.Uint32(data[base+12 : base+16])
		if int(records[i].origOff)+int(records[i].length) > len(data) {
			return nil, fmt.Errorf("table %s extends past EOF", string(records[i].tag[:]))
		}
	}

	// TTFs traditionally list tables in alphabetical order by tag. Sorting
	// is not strictly required but avoids surprising parsers that assume it.
	sort.Slice(records, func(i, j int) bool {
		return bytes.Compare(records[i].tag[:], records[j].tag[:]) < 0
	})

	// Lay out the new file: header + records + padded table data.
	searchRange, entrySelector, rangeShift := binarySearchParams(numTables)
	headerLen := 12 + 16*numTables
	var buf bytes.Buffer

	// Rough preallocation: header + sum of table lengths + alignment padding.
	totalLen := headerLen
	for _, r := range records {
		totalLen += int(r.length) + 3
	}
	buf.Grow(totalLen)

	// Write sfnt header + placeholder records; we fill offsets after we
	// know the post-header layout.
	header := make([]byte, headerLen)
	binary.BigEndian.PutUint32(header[0:4], sfntVersion)
	binary.BigEndian.PutUint16(header[4:6], uint16(numTables))
	binary.BigEndian.PutUint16(header[6:8], searchRange)
	binary.BigEndian.PutUint16(header[8:10], entrySelector)
	binary.BigEndian.PutUint16(header[10:12], rangeShift)

	buf.Write(header) // will rewrite in place later via buf.Bytes()

	// Append each table's data, 4-byte aligned. Record the new offsets.
	newOffsets := make([]uint32, len(records))
	for i, rec := range records {
		// 4-byte align current position within the output buffer.
		for buf.Len()%4 != 0 {
			buf.WriteByte(0)
		}
		newOffsets[i] = uint32(buf.Len())
		tableData := data[rec.origOff : rec.origOff+rec.length]
		// Zero the 'head' table's checkSumAdjustment (offset 8, 4 bytes) —
		// we're not recomputing the font-wide checksum, and PDF consumers
		// don't verify it.
		if string(rec.tag[:]) == "head" && rec.length >= 12 {
			modified := make([]byte, rec.length)
			copy(modified, tableData)
			binary.BigEndian.PutUint32(modified[8:12], 0)
			tableData = modified
		}
		buf.Write(tableData)
	}
	// Final 4-byte alignment so length is nicely divisible.
	for buf.Len()%4 != 0 {
		buf.WriteByte(0)
	}

	// Now rewrite the table records in place with the new offsets.
	out := buf.Bytes()
	for i, rec := range records {
		base := 12 + 16*i
		copy(out[base:base+4], rec.tag[:])
		binary.BigEndian.PutUint32(out[base+4:base+8], rec.checkSum)
		binary.BigEndian.PutUint32(out[base+8:base+12], newOffsets[i])
		binary.BigEndian.PutUint32(out[base+12:base+16], rec.length)
	}
	return out, nil
}

// binarySearchParams computes the sfnt Offset Table's searchRange /
// entrySelector / rangeShift fields for the given table count. See
// https://learn.microsoft.com/en-us/typography/opentype/spec/otff#table-directory.
func binarySearchParams(numTables int) (searchRange, entrySelector, rangeShift uint16) {
	pow2 := 1
	exp := 0
	for pow2*2 <= numTables {
		pow2 *= 2
		exp++
	}
	searchRange = uint16(pow2 * 16)
	entrySelector = uint16(exp)
	rangeShift = uint16(numTables*16) - searchRange
	return
}


