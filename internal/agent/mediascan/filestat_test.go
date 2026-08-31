//go:build unix

package mediascan

// The assertions here are about ownership, inodes and permission bits, none of
// which mean the same thing off a Unix filesystem, so the file is built only
// where they do.

import (
	"Metarr/internal/shared/scanmodel"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

// scanTreeWithSetup builds a tree inside a named item folder, hands the folder's
// path to setup so a test can adjust permissions or add a symlink, and then
// scans it. It is scanTree with a hook: the stat fields under test are the ones
// buildTree cannot express through its path-to-content map.
func scanTreeWithSetup(t *testing.T, folderName string, directoryType scanmodel.DirectoryType, files map[string]string, setup func(itemPath string)) *scanmodel.ScanResult {
	t.Helper()

	prefixed := make(map[string]string, len(files))
	for relativePath, content := range files {
		prefixed[folderName+"/"+relativePath] = content
	}
	root := buildTree(t, prefixed)
	itemPath := filepath.Join(root, folderName)

	if setup != nil {
		setup(itemPath)
	}

	result, err := Scan(itemPath, directoryType)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	return result
}

// TestSidecarStatRecordsPermissionsAndOwnership is the core assertion of this
// feature: a sidecar record carries what stat(2) reports about the file, not
// just its size and modification time.
func TestSidecarStatRecordsPermissionsAndOwnership(t *testing.T) {
	const posterContent = "poster bytes"

	result := scanTreeWithSetup(t, "The Movie (2019)", scanmodel.TypeMovie, map[string]string{
		"The Movie (2019).mkv": "video",
		"poster.jpg":           posterContent,
	}, func(itemPath string) {
		if err := os.Chmod(filepath.Join(itemPath, "poster.jpg"), 0o600); err != nil {
			t.Fatalf("chmod poster.jpg: %v", err)
		}
	})

	// Artwork named for its kind rather than for the film prefix-matches no
	// media file, so it belongs to the directory.
	sidecar := directorySidecarByName(t, result, "poster.jpg")

	stat := sidecar.Stat
	if stat == nil {
		t.Fatal("sidecar Stat = nil, want a stat record")
	}

	if stat.Mode != "-rw-------" {
		t.Errorf("Stat.Mode = %q, want %q", stat.Mode, "-rw-------")
	}
	if stat.ModeBits != 0o600 {
		t.Errorf("Stat.ModeBits = %#o, want %#o", stat.ModeBits, 0o600)
	}
	if stat.SizeBytes != int64(len(posterContent)) {
		t.Errorf("Stat.SizeBytes = %d, want %d", stat.SizeBytes, len(posterContent))
	}
	if !stat.ModifiedAt.AsTime().Equal(sidecar.ModifiedAt.AsTime()) {
		t.Errorf("Stat.ModifiedAt = %v, want the record's ModifiedAt %v", stat.ModifiedAt.AsTime(), sidecar.ModifiedAt.AsTime())
	}
	if stat.Uid != uint32(os.Getuid()) {
		t.Errorf("Stat.Uid = %d, want the running user %d", stat.Uid, os.Getuid())
	}
	if stat.Gid != uint32(os.Getgid()) {
		t.Errorf("Stat.Gid = %d, want the running group %d", stat.Gid, os.Getgid())
	}
	if stat.Inode == 0 {
		t.Error("Stat.Inode = 0, want the file's inode number")
	}
	if stat.LinkCount != 1 {
		t.Errorf("Stat.LinkCount = %d, want 1 for a freshly written file", stat.LinkCount)
	}
	if stat.AccessedAt == nil || stat.AccessedAt.AsTime().IsZero() {
		t.Error("Stat.AccessedAt is zero, want the file's access time")
	}
	if stat.ChangedAt == nil || stat.ChangedAt.AsTime().IsZero() {
		t.Error("Stat.ChangedAt is zero, want the file's status change time")
	}
	if stat.IsSymlink {
		t.Error("Stat.IsSymlink = true, want false for a regular file")
	}
}

// TestSidecarStatResolvesOwnerNames covers the lookup layered on top of the raw
// ids, which is what makes the record readable without a passwd file to hand.
func TestSidecarStatResolvesOwnerNames(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("cannot resolve the running user: %v", err)
	}

	result := scanTreeWithSetup(t, "The Movie (2019)", scanmodel.TypeMovie, map[string]string{
		"The Movie (2019).mkv": "video",
		"fanart.jpg":           "art",
	}, nil)

	stat := directorySidecarByName(t, result, "fanart.jpg").Stat
	if stat == nil {
		t.Fatal("sidecar Stat = nil, want a stat record")
	}

	if stat.OwnerName != currentUser.Username {
		t.Errorf("Stat.OwnerName = %q, want %q", stat.OwnerName, currentUser.Username)
	}

	// The group the file landed in is the process's, which may not be the one
	// the user record names, so it is resolved the same way the scanner does
	// rather than compared against currentUser.Gid.
	if group, err := user.LookupGroupId(strconv.Itoa(os.Getgid())); err == nil {
		if stat.GroupName != group.Name {
			t.Errorf("Stat.GroupName = %q, want %q", stat.GroupName, group.Name)
		}
	}
}

// TestMediaFileStatRecorded confirms the media file records carry the same block
// as the sidecars beside them.
func TestMediaFileStatRecorded(t *testing.T) {
	const episodeContent = "episode bytes"

	result := scanTreeWithSetup(t, "The Show", scanmodel.TypeTV, map[string]string{
		"Season 01/The Show S01E01.mkv": episodeContent,
	}, nil)

	mediaFile := mediaFileByName(t, result, "The Show S01E01.mkv")
	stat := mediaFile.Stat
	if stat == nil {
		t.Fatal("media file Stat = nil, want a stat record")
	}
	if stat.SizeBytes != int64(len(episodeContent)) {
		t.Errorf("Stat.SizeBytes = %d, want %d", stat.SizeBytes, len(episodeContent))
	}
	if stat.Inode == 0 {
		t.Error("Stat.Inode = 0, want the file's inode number")
	}
}

// TestSidecarStatFlagsSymlink checks that a linked sidecar is reported as the
// link it is. The scan walks with lstat, so recording the target's stat here
// would misdescribe both files.
func TestSidecarStatFlagsSymlink(t *testing.T) {
	result := scanTreeWithSetup(t, "The Movie (2019)", scanmodel.TypeMovie, map[string]string{
		"The Movie (2019).mkv": "video",
		"poster.jpg":           "art",
	}, func(itemPath string) {
		if err := os.Symlink(filepath.Join(itemPath, "poster.jpg"), filepath.Join(itemPath, "banner.jpg")); err != nil {
			t.Fatalf("linking banner.jpg: %v", err)
		}
	})

	linked := directorySidecarByName(t, result, "banner.jpg")
	if linked.Stat == nil {
		t.Fatal("linked sidecar Stat = nil, want a stat record")
	}
	if !linked.Stat.IsSymlink {
		t.Error("Stat.IsSymlink = false for a symlinked sidecar, want true")
	}

	regular := directorySidecarByName(t, result, "poster.jpg")
	if regular.Stat == nil {
		t.Fatal("regular sidecar Stat = nil, want a stat record")
	}
	if regular.Stat.IsSymlink {
		t.Error("Stat.IsSymlink = true for the link's target, want false")
	}
}

// TestDirectorySidecarStatRecorded covers the third owner a sidecar can have —
// the directory itself — since each is built by a different branch of
// classifySidecars.
func TestDirectorySidecarStatRecorded(t *testing.T) {
	result := scanTreeWithSetup(t, "The Show", scanmodel.TypeTV, map[string]string{
		"Season 01/The Show S01E01.mkv": "episode",
		"tvshow.nfo":                    "<tvshow><title>The Show</title></tvshow>",
		"Season 01/season01-poster.jpg": "art",
	}, nil)

	if stat := directorySidecarByName(t, result, "tvshow.nfo").Stat; stat == nil {
		t.Error("directory sidecar Stat = nil, want a stat record")
	} else if stat.Inode == 0 {
		t.Error("directory sidecar Stat.Inode = 0, want the file's inode number")
	}

	if stat := seasonSidecarByName(t, result, 1, "season01-poster.jpg").Stat; stat == nil {
		t.Error("season sidecar Stat = nil, want a stat record")
	} else if stat.Inode == 0 {
		t.Error("season sidecar Stat.Inode = 0, want the file's inode number")
	}
}
