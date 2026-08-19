//go:build !darwin && !linux

package mediascan

// The fallback for platforms whose stat structure this package does not know
// how to read. It exists so the scanner still compiles and runs anywhere, not
// because we ship to such a platform: development happens on macOS and the
// service runs on Linux.

import "io/fs"

// statSupportsOwnerIDs reports that no user or group ids are available here, so
// name resolution is skipped rather than resolving an id nothing supplied.
const statSupportsOwnerIDs = false

// fileStatFrom records what fs.FileInfo alone can say: mode, size, modification
// time and whether the entry is a symlink. The ownership, inode and block
// fields stay zero.
func fileStatFrom(info fs.FileInfo) *FileStat {
	return portableFileStat(info)
}
