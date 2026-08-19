package nfo

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// cp1252Replacements maps the bytes where Windows-1252 differs from
// ISO-8859-1: 0x80-0x9F, which Latin-1 reserves for C1 control characters and
// Windows-1252 uses for printable punctuation. utf8.RuneError stands in for
// the five positions Windows-1252 leaves undefined, since emitting a raw C1
// control byte would produce XML that is invalid by definition.
var cp1252Replacements = [32]rune{
	0x20AC, 0xFFFD, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021,
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0xFFFD, 0x017D, 0xFFFD,
	0xFFFD, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0xFFFD, 0x017E, 0x0178,
}

// newCharsetReader converts input to UTF-8 for the encoding named in an XML
// declaration. encoding/xml refuses to decode a document whose declared
// encoding it doesn't recognize, and plenty of NFO files in long-lived
// libraries were written as ISO-8859-1 or Windows-1252, so without this a
// scan would simply fail on them.
//
// Only single-byte Western encodings are handled, which is the realistic set
// for these files; taking on golang.org/x/net/html/charset to cover two
// codepages isn't a worthwhile dependency.
func newCharsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch normalizeCharsetName(charset) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return input, nil

	case "iso-8859-1", "iso8859-1", "latin1", "latin-1", "l1", "cp819", "windows-1252", "cp1252", "win-1252", "1252":
		// ISO-8859-1 is deliberately decoded as Windows-1252. Documents
		// labelled Latin-1 overwhelmingly contain 1252 punctuation in
		// practice, and the two agree everywhere else, so this is the reading
		// that recovers real text rather than replacement characters. It is
		// the same accommodation HTML5 mandates for the label.
		return newSingleByteReader(input)

	default:
		return nil, fmt.Errorf("nfo: unsupported document encoding %q", charset)
	}
}

// normalizeCharsetName lowercases the label and strips the quoting and
// whitespace that turn up in hand-edited XML declarations.
func normalizeCharsetName(charset string) string {
	trimmed := strings.TrimSpace(strings.ToLower(charset))
	return strings.Trim(trimmed, `"'`)
}

// newSingleByteReader reads all of input and returns its UTF-8 transcoding.
// NFO files are small sidecars, so buffering whole is simpler than a
// streaming transcoder and costs nothing meaningful.
func newSingleByteReader(input io.Reader) (io.Reader, error) {
	encoded, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}

	var decoded bytes.Buffer
	decoded.Grow(len(encoded))
	for _, encodedByte := range encoded {
		switch {
		case encodedByte < 0x80:
			decoded.WriteByte(encodedByte)
		case encodedByte < 0xA0:
			decoded.WriteRune(cp1252Replacements[encodedByte-0x80])
		default:
			decoded.WriteRune(rune(encodedByte))
		}
	}
	return &decoded, nil
}
