package nfo

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// xmlDeclaration matches what Kodi itself writes at the top of an NFO file.
const xmlDeclaration = `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>` + "\n"

// xmlIndent is four spaces, again matching Kodi's own output, so a file Metarr
// rewrites doesn't show up as reformatted noise in a diff.
const xmlIndent = "    "

// ErrMultipleEpisodes is returned when asked to marshal a Document holding
// more than one episode. Kodi v22 dropped support for several
// <episodedetails> roots in one file, so writing that layout back out would
// produce a file newer Kodi versions ignore. Callers should write one file per
// episode, named with the -SxxEyy suffix.
var ErrMultipleEpisodes = errors.New("nfo: cannot marshal more than one episode into a single document; write one file per episode")

// Marshal renders doc as a Kodi NFO file, always with a single root element.
func Marshal(doc *Document) ([]byte, error) {
	if doc == nil {
		return nil, errors.New("nfo: cannot marshal a nil document")
	}

	var root any
	switch {
	case doc.Movie != nil:
		restoreUnknownElementNames(doc.Movie.Extra)
		root = doc.Movie
	case doc.TVShow != nil:
		restoreUnknownElementNames(doc.TVShow.Extra)
		root = doc.TVShow
	case doc.MusicVideo != nil:
		restoreUnknownElementNames(doc.MusicVideo.Extra)
		root = doc.MusicVideo
	case len(doc.Episodes) > 1:
		return nil, ErrMultipleEpisodes
	case len(doc.Episodes) == 1:
		restoreUnknownElementNames(doc.Episodes[0].Extra)
		root = &doc.Episodes[0]
	default:
		return nil, fmt.Errorf("nfo: document of kind %q has no content to marshal", doc.Kind)
	}

	body, err := xml.MarshalIndent(root, "", xmlIndent)
	if err != nil {
		return nil, fmt.Errorf("nfo: encoding document: %w", err)
	}

	var out bytes.Buffer
	out.Grow(len(xmlDeclaration) + len(body) + 1)
	out.WriteString(xmlDeclaration)
	out.Write(body)
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// WriteFile renders doc and replaces the file at path with it atomically: the
// content is written to a temporary file in the same directory and then
// renamed over the destination. NFO files are Metarr's system of record, so a
// crash or full disk partway through must never be able to leave a truncated
// file where valid metadata used to be.
func WriteFile(path string, doc *Document) error {
	data, err := Marshal(doc)
	if err != nil {
		return err
	}

	// The temporary file must share a directory with the destination, since
	// os.Rename is only atomic within a filesystem.
	directory := filepath.Dir(path)
	tempFile, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("nfo: creating temporary file in %s: %w", directory, err)
	}
	tempPath := tempFile.Name()

	// Any failure from here on must clean up the temporary file rather than
	// leaving litter beside the user's media.
	cleanup := func() {
		tempFile.Close()
		os.Remove(tempPath)
	}

	if _, err := tempFile.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("nfo: writing %s: %w", tempPath, err)
	}
	if err := tempFile.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("nfo: syncing %s: %w", tempPath, err)
	}
	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("nfo: closing %s: %w", tempPath, err)
	}

	// Carry over the existing file's permissions so a rewrite doesn't change
	// how the library is accessed. A new file keeps CreateTemp's 0600 tightened
	// to the usual 0644 for media sidecars.
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.Chmod(tempPath, mode); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("nfo: setting mode on %s: %w", tempPath, err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("nfo: replacing %s: %w", path, err)
	}
	return nil
}
