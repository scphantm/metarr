//go:build darwin

package mediascan

// macOS names its stat timespec fields Atimespec/Mtimespec/Ctimespec and does
// expose a birth time, which is why this cannot share a file with Linux even
// though the two produce the same record.

import (
	"io/fs"
	"syscall"
	"time"
)

// statSupportsOwnerIDs reports that the stat structure on this platform carries
// real user and group ids, so resolving them to names is meaningful.
const statSupportsOwnerIDs = true

// fileStatFrom builds the stat record for one walked file. The FileInfo already
// holds the result of the lstat the directory walk performed, so nothing here
// touches the filesystem again.
//
// Every assignment casts explicitly: the field widths in syscall.Stat_t differ
// between architectures, and an untyped assignment would compile on one and
// break on the next.
func fileStatFrom(info fs.FileInfo) *FileStat {
	fileStat := portableFileStat(info)

	systemStat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileStat
	}

	fileStat.ModeBits = uint32(systemStat.Mode) & 0o7777
	fileStat.UID = systemStat.Uid
	fileStat.GID = systemStat.Gid
	fileStat.Inode = systemStat.Ino
	fileStat.LinkCount = uint64(systemStat.Nlink)
	fileStat.DeviceID = uint64(systemStat.Dev)
	fileStat.BlockSize = int64(systemStat.Blksize)
	fileStat.Blocks = systemStat.Blocks
	fileStat.AccessedAt = timespecToTime(systemStat.Atimespec)
	fileStat.ChangedAt = timespecToTime(systemStat.Ctimespec)
	fileStat.CreatedAt = timespecToTime(systemStat.Birthtimespec)

	return fileStat
}

// timespecToTime converts a stat timestamp to UTC, matching how the scanner
// stores every other time so stored records compare across hosts.
func timespecToTime(timespec syscall.Timespec) time.Time {
	return time.Unix(timespec.Sec, timespec.Nsec).UTC()
}
