package agentregistry

import (
	"strings"
	"testing"

	"Metarr/internal/shared/scanmodel"
)

func TestPathTranslatesFromAgentRootToServerRoot(t *testing.T) {
	translator := NewPathTranslator("/mnt/tank/movies", "/media/movies")

	cases := map[string]string{
		"/mnt/tank/movies":                        "/media/movies",
		"/mnt/tank/movies/":                       "/media/movies",
		"/mnt/tank/movies/Blade Runner (1982)":    "/media/movies/Blade Runner (1982)",
		"/mnt/tank/movies/Show/Season 01/ep.mkv":  "/media/movies/Show/Season 01/ep.mkv",
		"/mnt/tank/movies/nested/../Blade Runner": "/media/movies/Blade Runner",
	}

	for agentPath, want := range cases {
		got, err := translator.Path(agentPath)
		if err != nil {
			t.Errorf("Path(%q) error = %v", agentPath, err)
			continue
		}
		if got != want {
			t.Errorf("Path(%q) = %q, want %q", agentPath, got, want)
		}
	}
}

// A path outside the mapped root means the agent reported something from a
// library it was not asked about. Storing it would mix two machines'
// filesystems in one collection, so it has to fail loudly here.
func TestPathRejectsAnythingOutsideTheAgentRoot(t *testing.T) {
	translator := NewPathTranslator("/mnt/tank/movies", "/media/movies")

	for _, agentPath := range []string{
		"/mnt/tank/tv/Show",
		"/etc/passwd",
		"/mnt/tank",
		"/mnt/tank/movies/../tv/Show",
		// A sibling whose name merely starts with the root's must not pass.
		"/mnt/tank/movies-backup/Show",
	} {
		if got, err := translator.Path(agentPath); err == nil {
			t.Errorf("Path(%q) = %q, want an error", agentPath, got)
		}
	}
}

// Roots with trailing separators are easy to configure by accident and must
// behave identically to ones without.
func TestPathToleratesTrailingSeparatorsOnEitherRoot(t *testing.T) {
	translator := NewPathTranslator("/mnt/tank/movies/", "/media/movies/")

	got, err := translator.Path("/mnt/tank/movies/Blade Runner (1982)")
	if err != nil {
		t.Fatalf("Path error = %v", err)
	}
	if want := "/media/movies/Blade Runner (1982)"; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func scanResultUnderAgentRoot() *scanmodel.ScanResult {
	return &scanmodel.ScanResult{
		Directory: &scanmodel.TVSeries{
			Path:         "/mnt/tank/movies/Show",
			ScanRootPath: "/mnt/tank/movies",
			// Sidecars and seasons carry relative paths only, so they need no
			// translation. They are populated here so the test would notice if
			// that ever changed and they started holding absolute paths.
			Sidecars: []*scanmodel.SidecarFile{
				{RelativePath: "poster.jpg", FileName: "poster.jpg"},
			},
			Seasons: []*scanmodel.TVSeason{{
				SeasonNumber: 1,
				FolderName:   "Season 01",
				Sidecars: []*scanmodel.SidecarFile{
					{RelativePath: "Season 01/season.nfo", FileName: "season.nfo"},
				},
			}},
		},
		MediaFiles: []*scanmodel.MediaFile{{
			Path:          "/mnt/tank/movies/Show/Season 01/ep.mkv",
			DirectoryPath: "/mnt/tank/movies/Show",
			ScanRootPath:  "/mnt/tank/movies",
			RelativePath:  "Season 01/ep.mkv",
			Sidecars: []*scanmodel.SidecarFile{
				{RelativePath: "Season 01/ep.nfo", FileName: "ep.nfo"},
			},
		}},
	}
}

// Every path in a result has to be translated. A record left half-translated
// would look correct in a listing and fail on every read.
func TestResultTranslatesEveryPathItCarries(t *testing.T) {
	translator := NewPathTranslator("/mnt/tank/movies", "/media/movies")
	result := scanResultUnderAgentRoot()

	if err := translator.Result(result); err != nil {
		t.Fatalf("Result error = %v", err)
	}

	absolute := []string{
		result.Directory.Path,
		result.Directory.ScanRootPath,
		result.MediaFiles[0].Path,
		result.MediaFiles[0].DirectoryPath,
		result.MediaFiles[0].ScanRootPath,
	}

	for _, path := range absolute {
		if strings.HasPrefix(path, "/mnt/tank") {
			t.Errorf("path was left in the agent's terms: %q", path)
		}
		if !strings.HasPrefix(path, "/media/movies") {
			t.Errorf("path is not under the server root: %q", path)
		}
	}
}

func TestResultRejectsAnItemWithAForeignPath(t *testing.T) {
	translator := NewPathTranslator("/mnt/tank/movies", "/media/movies")

	result := scanResultUnderAgentRoot()
	result.MediaFiles[0].Path = "/etc/passwd"

	if err := translator.Result(result); err == nil {
		t.Error("Result accepted an item carrying a path outside the library")
	}
}

// Empty paths are absent values, not the filesystem root, and must be left
// alone rather than translated into the server root.
func TestResultLeavesEmptyOptionalPathsAlone(t *testing.T) {
	translator := NewPathTranslator("/mnt/tank/movies", "/media/movies")

	result := scanResultUnderAgentRoot()
	result.Directory.ScanRootPath = ""
	result.MediaFiles[0].DirectoryPath = ""

	if err := translator.Result(result); err != nil {
		t.Fatalf("Result error = %v", err)
	}
	if result.Directory.ScanRootPath != "" {
		t.Errorf("empty scan root became %q", result.Directory.ScanRootPath)
	}
	if result.MediaFiles[0].DirectoryPath != "" {
		t.Errorf("empty directory path became %q", result.MediaFiles[0].DirectoryPath)
	}
}

// Later parts of a split item carry media files with no directory record.
func TestResultHandlesAPartWithNoDirectory(t *testing.T) {
	translator := NewPathTranslator("/mnt/tank/movies", "/media/movies")

	result := &scanmodel.ScanResult{
		MediaFiles: []*scanmodel.MediaFile{{Path: "/mnt/tank/movies/Show/ep.mkv"}},
	}

	if err := translator.Result(result); err != nil {
		t.Fatalf("Result error = %v", err)
	}
	if want := "/media/movies/Show/ep.mkv"; result.MediaFiles[0].Path != want {
		t.Errorf("path = %q, want %q", result.MediaFiles[0].Path, want)
	}
}

// Sidecars, seasons and relative paths describe a file's position inside its
// own item, so they already mean the same thing on both machines. Translating
// them would corrupt them.
func TestResultLeavesRelativePathsUntouched(t *testing.T) {
	translator := NewPathTranslator("/mnt/tank/movies", "/media/movies")
	result := scanResultUnderAgentRoot()

	if err := translator.Result(result); err != nil {
		t.Fatalf("Result error = %v", err)
	}

	unchanged := map[string]string{
		"directory sidecar": result.Directory.Sidecars[0].RelativePath,
		"season sidecar":    result.Directory.Seasons[0].Sidecars[0].RelativePath,
		"media file":        result.MediaFiles[0].RelativePath,
		"media sidecar":     result.MediaFiles[0].Sidecars[0].RelativePath,
	}
	for name, path := range unchanged {
		if strings.HasPrefix(path, "/") {
			t.Errorf("%s became absolute: %q", name, path)
		}
	}
	if got := result.Directory.Sidecars[0].RelativePath; got != "poster.jpg" {
		t.Errorf("directory sidecar relative path = %q, want poster.jpg", got)
	}
}

// An agent may run on Windows while this server runs on Linux, so the agent
// side of a translation cannot go through path/filepath — which is compiled
// for the server's OS and would treat a backslash as an ordinary character.

func TestPathTranslatesWindowsAgentPaths(t *testing.T) {
	translator := NewPathTranslator(`D:\Media\Movies`, "/media/movies")

	cases := map[string]string{
		`D:\Media\Movies`:                       "/media/movies",
		`D:\Media\Movies\Blade Runner (1982)`:   "/media/movies/Blade Runner (1982)",
		`D:\Media\Movies\Show\Season 01\ep.mkv`: "/media/movies/Show/Season 01/ep.mkv",
		// Windows accepts forward slashes too, so an agent may report either.
		`D:/Media/Movies/Show/ep.mkv`: "/media/movies/Show/ep.mkv",
		// Case-insensitive, as the filesystem it came from is.
		`d:\media\movies\Show\ep.mkv`: "/media/movies/Show/ep.mkv",
	}

	for agentPath, want := range cases {
		got, err := translator.Path(agentPath)
		if err != nil {
			t.Errorf("Path(%q) error = %v", agentPath, err)
			continue
		}
		if got != want {
			t.Errorf("Path(%q) = %q, want %q", agentPath, got, want)
		}
	}
}

func TestPathRejectsWindowsPathsOutsideTheRoot(t *testing.T) {
	translator := NewPathTranslator(`D:\Media\Movies`, "/media/movies")

	for _, agentPath := range []string{
		`D:\Media\TV\Show`,
		`C:\Windows\System32`,
		`D:\Media`,
		`D:\Media\Movies\..\TV\Show`,
		`D:\Media\Movies-backup\Show`,
	} {
		if got, err := translator.Path(agentPath); err == nil {
			t.Errorf("Path(%q) = %q, want an error", agentPath, got)
		}
	}
}

func TestPathTranslatesUNCAgentPaths(t *testing.T) {
	translator := NewPathTranslator(`\\nas\media\movies`, "/media/movies")

	got, err := translator.Path(`\\nas\media\movies\Blade Runner (1982)\movie.mkv`)
	if err != nil {
		t.Fatalf("Path error = %v", err)
	}
	if want := "/media/movies/Blade Runner (1982)/movie.mkv"; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

// A POSIX agent root must stay case-sensitive: two files differing only in
// case are two different files there, and folding them would collide records.
func TestPathKeepsPosixAgentPathsCaseSensitive(t *testing.T) {
	translator := NewPathTranslator("/mnt/tank/movies", "/media/movies")

	if got, err := translator.Path("/MNT/TANK/MOVIES/Show"); err == nil {
		t.Errorf("Path(%q) = %q, want an error on a POSIX agent", "/MNT/TANK/MOVIES/Show", got)
	}
}
