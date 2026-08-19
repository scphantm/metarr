package agentregistry

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"Metarr/internal/shared/scanmodel"
)

// PathTranslator rewrites the paths in a scan result from the agent's view of
// a library to the server's.
//
// The library is stored under the server's own paths regardless of which agent
// scanned it. That is what keeps the unique index on path meaningful:
// rescanning from a different agent, or remapping an agent to a new mount
// point, updates the existing records instead of creating a second, parallel
// copy of the same library under a different name.
//
// The agent may be running on a different operating system than this server —
// a Windows agent reporting D:\Media\Movies to a Linux server is a supported
// deployment. So the agent side of the translation is done in the agent's own
// terms rather than with path/filepath, which is compiled for whichever OS the
// *server* happens to be on and would treat a backslash as an ordinary
// character. Only the server side of a translated path goes through filepath.
type PathTranslator struct {
	// agentRoot is normalised to forward slashes for comparison. Windows
	// accepts both separators, so this loses nothing.
	agentRoot      string
	serverRoot     string
	agentIsWindows bool
}

// windowsRoot matches a drive-letter path (D:\Media) or a UNC share
// (\\nas\media), which are the two shapes a Windows library root takes.
var windowsRoot = regexp.MustCompile(`^([A-Za-z]:|\\\\)`)

// NewPathTranslator returns a translator between the two roots naming the same
// library. The agent's operating system is inferred from the shape of its root
// rather than configured, since a path is the only thing the agent is asked
// for.
func NewPathTranslator(agentRoot, serverRoot string) PathTranslator {
	isWindows := windowsRoot.MatchString(agentRoot) || strings.Contains(agentRoot, `\`)

	return PathTranslator{
		agentRoot:      normalizeAgentPath(agentRoot),
		serverRoot:     filepath.Clean(serverRoot),
		agentIsWindows: isWindows,
	}
}

// normalizeAgentPath puts an agent path into a single comparable form:
// forward slashes, no redundant separators, no . or .. segments.
//
// path.Clean rather than filepath.Clean on purpose — it is the forward-slash
// implementation regardless of what this server is compiled for, which is
// exactly what is needed for a path that came from somewhere else.
func normalizeAgentPath(p string) string {
	normalized := strings.ReplaceAll(p, `\`, "/")
	normalized = path.Clean(normalized)
	return strings.TrimSuffix(normalized, "/")
}

// Path rewrites one path. A path that does not sit under the agent's root is an
// error rather than something to pass through: storing it would mix two
// machines' filesystems in one collection, and the mistake would only surface
// much later as a record nobody can find.
func (t PathTranslator) Path(agentPath string) (string, error) {
	cleaned := normalizeAgentPath(agentPath)

	if t.equal(cleaned, t.agentRoot) {
		return t.serverRoot, nil
	}

	prefix := t.agentRoot + "/"
	if !t.hasPrefix(cleaned, prefix) {
		return "", fmt.Errorf("path %q is not under the agent's library root %q", agentPath, t.agentRoot)
	}

	// The remainder is in the agent's terms; joining it onto the server root
	// with filepath converts it to this machine's separators.
	relative := cleaned[len(prefix):]
	return filepath.Join(t.serverRoot, filepath.FromSlash(relative)), nil
}

// Windows filesystems are case-insensitive, so a root typed as D:\Media must
// still match a path the agent reported as d:\media.
func (t PathTranslator) equal(a, b string) bool {
	if t.agentIsWindows {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func (t PathTranslator) hasPrefix(value, prefix string) bool {
	if len(value) < len(prefix) {
		return false
	}
	return t.equal(value[:len(prefix)], prefix)
}

// Result rewrites every absolute path in a scan result in place.
//
// Only five fields hold absolute paths: a directory's own path and scan root,
// and a media file's path, directory path and scan root. Everything else the
// scan produces — sidecars, seasons, the media file's own relative path — is
// stored relative to its parent, so it already means the same thing on both
// machines and is deliberately left alone.
//
// Every absolute path is translated or the whole item is rejected. A
// half-translated record — a directory under the server's path holding media
// files under the agent's — would be worse than no record at all, because it
// would look correct in a listing and fail on every read.
func (t PathTranslator) Result(result *scanmodel.ScanResult) error {
	if result == nil {
		return nil
	}

	if directory := result.Directory; directory != nil {
		if err := t.rewrite(&directory.Path); err != nil {
			return err
		}
		if err := t.rewriteIfSet(&directory.ScanRootPath); err != nil {
			return err
		}
	}

	for i := range result.MediaFiles {
		mediaFile := &result.MediaFiles[i]
		if err := t.rewrite(&mediaFile.Path); err != nil {
			return err
		}
		if err := t.rewriteIfSet(&mediaFile.DirectoryPath); err != nil {
			return err
		}
		if err := t.rewriteIfSet(&mediaFile.ScanRootPath); err != nil {
			return err
		}
	}

	return nil
}

func (t PathTranslator) rewrite(path *string) error {
	translated, err := t.Path(*path)
	if err != nil {
		return err
	}
	*path = translated
	return nil
}

// rewriteIfSet skips empty paths, which are absent values rather than paths
// pointing at the filesystem root.
func (t PathTranslator) rewriteIfSet(path *string) error {
	if *path == "" {
		return nil
	}
	return t.rewrite(path)
}
