package nfo

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"Metarr/internal/shared/metadata"
)

// utf8BOM is stripped before parsing; encoding/xml treats a leading BOM as
// character data and fails on it.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// document is a parsed NFO file. Exactly one of the typed fields is populated,
// according to Kind — except for Episodes, which holds more than one entry
// when the file used the legacy multi-episode layout.
type document struct {
	Kind       metadata.DocumentKind
	Movie      *movie
	TVShow     *tvShow
	Episodes   []episodeDetails
	MusicVideo *musicVideo
}

// ReadFile reads the NFO file at path and returns it as a metadata.Metadata.
// I/O failures and genuinely malformed XML are errors; a file whose shape isn't
// recognized comes back as metadata with Kind KindURL or KindUnknown, so a
// library scan can record the oddity and carry on.
func ReadFile(path string) (*metadata.Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := parse(data)
	if err != nil {
		return nil, err
	}
	return doc.toMetadata(), nil
}

// parse reads an NFO document out of data.
//
// The decode is a token loop rather than a single xml.Unmarshal because Kodi
// v21 and earlier wrote multi-episode NFOs as several <episodedetails> roots
// concatenated into one file. That is not well-formed XML — it has no single
// root element — and xml.Unmarshal rejects it outright, which would silently
// lose the metadata for every episode in such a file. Walking tokens accepts
// both that layout and ordinary single-root documents.
func parse(data []byte) (*document, error) {
	trimmed := bytes.TrimPrefix(data, utf8BOM)

	// A file that doesn't open with a tag isn't XML at all. Kodi accepts an
	// .nfo containing just a scraper URL, so report the shape instead of
	// failing to parse it.
	if !bytes.HasPrefix(bytes.TrimLeft(trimmed, " \t\r\n"), []byte("<")) {
		if len(bytes.TrimSpace(trimmed)) == 0 {
			return &document{Kind: metadata.KindUnknown}, nil
		}
		return &document{Kind: metadata.KindURL}, nil
	}

	decoder := xml.NewDecoder(bytes.NewReader(trimmed))
	decoder.CharsetReader = newCharsetReader
	// Kodi writes bare HTML entities such as &nbsp; into plot text. Without an
	// entity table the decoder errors on them, so map anything unrecognized to
	// nothing rather than discarding the whole document over a stray entity.
	decoder.Strict = false

	doc := &document{Kind: metadata.KindUnknown}
	foundRoot := false

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("nfo: parsing document: %w", err)
		}

		startElement, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		switch strings.ToLower(startElement.Name.Local) {
		case "movie":
			var parsed movie
			if err := decoder.DecodeElement(&parsed, &startElement); err != nil {
				return nil, fmt.Errorf("nfo: decoding <movie>: %w", err)
			}
			doc.Movie = &parsed
			setKindOnce(doc, metadata.KindMovie, &foundRoot)

		case "tvshow":
			var parsed tvShow
			if err := decoder.DecodeElement(&parsed, &startElement); err != nil {
				return nil, fmt.Errorf("nfo: decoding <tvshow>: %w", err)
			}
			doc.TVShow = &parsed
			setKindOnce(doc, metadata.KindTVShow, &foundRoot)

		case "episodedetails":
			var parsed episodeDetails
			if err := decoder.DecodeElement(&parsed, &startElement); err != nil {
				return nil, fmt.Errorf("nfo: decoding <episodedetails>: %w", err)
			}
			doc.Episodes = append(doc.Episodes, parsed)
			setKindOnce(doc, metadata.KindEpisode, &foundRoot)

		case "musicvideo":
			var parsed musicVideo
			if err := decoder.DecodeElement(&parsed, &startElement); err != nil {
				return nil, fmt.Errorf("nfo: decoding <musicvideo>: %w", err)
			}
			doc.MusicVideo = &parsed
			setKindOnce(doc, metadata.KindMusicVideo, &foundRoot)

		default:
			// Some unrelated root element. Skip its subtree so the loop can
			// still find a recognized document alongside it.
			if err := decoder.Skip(); err != nil {
				return nil, fmt.Errorf("nfo: skipping <%s>: %w", startElement.Name.Local, err)
			}
		}
	}

	// Mirror each preserved element's name into a plain string field, so the
	// tag still round-trips after the document has been through storage that
	// doesn't keep an xml.Name.
	switch {
	case doc.Movie != nil:
		captureUnknownElementNames(doc.Movie.Extra)
	case doc.TVShow != nil:
		captureUnknownElementNames(doc.TVShow.Extra)
	case doc.MusicVideo != nil:
		captureUnknownElementNames(doc.MusicVideo.Extra)
	}
	for i := range doc.Episodes {
		captureUnknownElementNames(doc.Episodes[i].Extra)
	}

	return doc, nil
}

// setKindOnce records the document's kind from the first recognized root
// element, so a file mixing types keeps the identity of what it led with.
func setKindOnce(doc *document, kind metadata.DocumentKind, foundRoot *bool) {
	if !*foundRoot {
		doc.Kind = kind
		*foundRoot = true
	}
}
