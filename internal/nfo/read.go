package nfo

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// DocumentKind names the root element a file actually turned out to contain.
type DocumentKind string

const (
	KindMovie      DocumentKind = "movie"
	KindTVShow     DocumentKind = "tvshow"
	KindEpisode    DocumentKind = "episodedetails"
	KindMusicVideo DocumentKind = "musicvideo"
	// KindURL is an .nfo holding a bare scraper URL rather than XML, a form
	// Kodi has historically accepted. There is nothing to parse, but the file
	// is still recognized rather than reported as broken.
	KindURL DocumentKind = "url"
	// KindUnknown is well-formed XML whose root element isn't one of the four
	// media document types.
	KindUnknown DocumentKind = "unknown"
)

// utf8BOM is stripped before parsing; encoding/xml treats a leading BOM as
// character data and fails on it.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Document is a parsed NFO file. Exactly one of the typed fields is populated,
// according to Kind — except for Episodes, which holds more than one entry
// when the file used the legacy multi-episode layout.
type Document struct {
	Kind       DocumentKind     `bson:"kind" json:"kind"`
	Movie      *Movie           `bson:"movie,omitempty" json:"movie,omitempty"`
	TVShow     *TVShow          `bson:"tvshow,omitempty" json:"tvshow,omitempty"`
	Episodes   []EpisodeDetails `bson:"episodes,omitempty" json:"episodes,omitempty"`
	MusicVideo *MusicVideo      `bson:"musicvideo,omitempty" json:"musicvideo,omitempty"`
}

// ReadFile parses the NFO file at path. Errors are reported only for I/O
// failures and genuinely malformed XML; a file whose shape simply isn't
// recognized comes back as a Document with Kind set to KindURL or KindUnknown,
// so callers scanning a library can record the oddity and carry on.
func ReadFile(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse reads an NFO document out of data.
//
// The decode is a token loop rather than a single xml.Unmarshal because Kodi
// v21 and earlier wrote multi-episode NFOs as several <episodedetails> roots
// concatenated into one file. That is not well-formed XML — it has no single
// root element — and xml.Unmarshal rejects it outright, which would silently
// lose the metadata for every episode in such a file. Walking tokens accepts
// both that layout and ordinary single-root documents.
func Parse(data []byte) (*Document, error) {
	trimmed := bytes.TrimPrefix(data, utf8BOM)

	// A file that doesn't open with a tag isn't XML at all. Kodi accepts an
	// .nfo containing just a scraper URL, so report the shape instead of
	// failing to parse it.
	if !bytes.HasPrefix(bytes.TrimLeft(trimmed, " \t\r\n"), []byte("<")) {
		if len(bytes.TrimSpace(trimmed)) == 0 {
			return &Document{Kind: KindUnknown}, nil
		}
		return &Document{Kind: KindURL}, nil
	}

	decoder := xml.NewDecoder(bytes.NewReader(trimmed))
	decoder.CharsetReader = newCharsetReader
	// Kodi writes bare HTML entities such as &nbsp; into plot text. Without an
	// entity table the decoder errors on them, so map anything unrecognized to
	// nothing rather than discarding the whole document over a stray entity.
	decoder.Strict = false

	document := &Document{Kind: KindUnknown}
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
			var movie Movie
			if err := decoder.DecodeElement(&movie, &startElement); err != nil {
				return nil, fmt.Errorf("nfo: decoding <movie>: %w", err)
			}
			document.Movie = &movie
			setKindOnce(document, KindMovie, &foundRoot)

		case "tvshow":
			var show TVShow
			if err := decoder.DecodeElement(&show, &startElement); err != nil {
				return nil, fmt.Errorf("nfo: decoding <tvshow>: %w", err)
			}
			document.TVShow = &show
			setKindOnce(document, KindTVShow, &foundRoot)

		case "episodedetails":
			var episode EpisodeDetails
			if err := decoder.DecodeElement(&episode, &startElement); err != nil {
				return nil, fmt.Errorf("nfo: decoding <episodedetails>: %w", err)
			}
			document.Episodes = append(document.Episodes, episode)
			setKindOnce(document, KindEpisode, &foundRoot)

		case "musicvideo":
			var musicVideo MusicVideo
			if err := decoder.DecodeElement(&musicVideo, &startElement); err != nil {
				return nil, fmt.Errorf("nfo: decoding <musicvideo>: %w", err)
			}
			document.MusicVideo = &musicVideo
			setKindOnce(document, KindMusicVideo, &foundRoot)

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
	case document.Movie != nil:
		captureUnknownElementNames(document.Movie.Extra)
	case document.TVShow != nil:
		captureUnknownElementNames(document.TVShow.Extra)
	case document.MusicVideo != nil:
		captureUnknownElementNames(document.MusicVideo.Extra)
	}
	for i := range document.Episodes {
		captureUnknownElementNames(document.Episodes[i].Extra)
	}

	return document, nil
}

// setKindOnce records the document's kind from the first recognized root
// element, so a file mixing types keeps the identity of what it led with.
func setKindOnce(document *Document, kind DocumentKind, foundRoot *bool) {
	if !*foundRoot {
		document.Kind = kind
		*foundRoot = true
	}
}
