package runtime

import (
	"path/filepath"
	"testing"
)

// These two functions are the only thing standing between a metadata endpoint
// and an arbitrary file read on the machine that holds the library. They run on
// the agent now, where the filesystem they guard is the real one, so the escape
// cases matter more here than they did on the server.

func TestResolveUnderAcceptsPathsInsideTheRoot(t *testing.T) {
	root := filepath.FromSlash("/mnt/tank/movies")

	cases := map[string]string{
		"":                      root,
		"Blade Runner (1982)":   filepath.Join(root, "Blade Runner (1982)"),
		"Show/Season 01":        filepath.Join(root, "Show", "Season 01"),
		"./Blade Runner (1982)": filepath.Join(root, "Blade Runner (1982)"),
		"Show/../Other":         filepath.Join(root, "Other"),
	}

	for relative, want := range cases {
		got, err := resolveUnder(root, relative)
		if err != nil {
			t.Errorf("resolveUnder(%q) error = %v", relative, err)
			continue
		}
		if got != want {
			t.Errorf("resolveUnder(%q) = %q, want %q", relative, got, want)
		}
	}
}

func TestResolveUnderRejectsEscapes(t *testing.T) {
	root := filepath.FromSlash("/mnt/tank/movies")

	for _, relative := range []string{
		"..",
		"../etc",
		"../../../../etc",
		"Show/../../../etc",
		filepath.FromSlash("/etc/passwd"),
	} {
		if got, err := resolveUnder(root, relative); err == nil {
			t.Errorf("resolveUnder(%q) = %q, want an error", relative, got)
		}
	}
}

func TestResolveWithinDirectoryAcceptsNFOFiles(t *testing.T) {
	directory := filepath.FromSlash("/mnt/tank/movies/Blade Runner (1982)")

	for _, name := range []string{"movie.nfo", "Movie.NFO", "extras/behind.nfo"} {
		got, err := resolveWithinDirectory(directory, name)
		if err != nil {
			t.Errorf("resolveWithinDirectory(%q) error = %v", name, err)
			continue
		}
		if want := filepath.Join(directory, filepath.FromSlash(name)); got != want {
			t.Errorf("resolveWithinDirectory(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestResolveWithinDirectoryRejectsEscapesAndNonNFO(t *testing.T) {
	directory := filepath.FromSlash("/mnt/tank/movies/Blade Runner (1982)")

	cases := map[string]string{
		"absolute path":        filepath.FromSlash("/etc/passwd"),
		"absolute nfo":         filepath.FromSlash("/etc/passwd.nfo"),
		"traversal":            "../../../../etc/passwd.nfo",
		"traversal mid-path":   "extras/../../../secrets.nfo",
		"not an nfo":           "movie.mkv",
		"no extension":         "movie",
		"nfo-lookalike suffix": "movie.nfo.txt",
	}

	for name, requested := range cases {
		if got, err := resolveWithinDirectory(directory, requested); err == nil {
			t.Errorf("%s: resolveWithinDirectory(%q) = %q, want an error", name, requested, got)
		}
	}
}

func TestIsWithinTreatsTheRootItselfAsInside(t *testing.T) {
	root := filepath.FromSlash("/mnt/tank/movies")

	if !isWithin(root, root) {
		t.Error("a root is not reported as inside itself")
	}
	if !isWithin(root, filepath.Join(root, "a", "b")) {
		t.Error("a nested path is not reported as inside")
	}
	// A sibling whose name merely starts with the root's must not pass.
	if isWithin(root, filepath.FromSlash("/mnt/tank/movies-backup/x")) {
		t.Error("a sibling sharing a name prefix was reported as inside")
	}
}
