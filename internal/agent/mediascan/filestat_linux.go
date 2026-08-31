//go:build linux

package mediascan

// Linux names its stat timespec fields Atim/Mtim/Ctim and exposes no birth time
// through stat(2), which is why this cannot share a file with macOS even though
// the two produce the same record. CreatedAt is left zero here rather than
// filled with the modification time, so "unknown" stays distinguishable from
// "created when it was last written".

import (
	"io/fs"
	"syscall"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"Metarr/internal/shared/scanmodel"
)

// statSupportsOwnerIDs reports that the stat structure on this platform carries
// real user and group ids, so resolving them to names is meaningful.
const statSupportsOwnerIDs = true

// fileStatFrom builds the stat record for one walked file. The scanmodel.FileInfo already
// holds the result of the lstat the directory walk performed, so nothing here
// touches the filesystem again.
//
// Every assignment casts explicitly: the field widths in syscall.Stat_t differ
// between architectures — Nlink and Blksize are narrower on arm64 than on amd64
// — and an untyped assignment would compile on one and break on the next.
func fileStatFrom(info fs.FileInfo) *scanmodel.FileStat {
	fileStat := portableFileStat(info)

	systemStat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileStat
	}

	fileStat.ModeBits = uint32(systemStat.Mode) & 0o7777
	fileStat.Uid = systemStat.Uid
	fileStat.Gid = systemStat.Gid
	fileStat.Inode = systemStat.Ino
	fileStat.LinkCount = uint64(systemStat.Nlink)
	fileStat.DeviceId = uint64(systemStat.Dev)
	fileStat.BlockSize = int64(systemStat.Blksize)
	fileStat.Blocks = systemStat.Blocks
	fileStat.AccessedAt = timespecToTimestamp(systemStat.Atim)
	fileStat.ChangedAt = timespecToTimestamp(systemStat.Ctim)

	return fileStat
}

// timespecToTimestamp converts a stat timestamp to a UTC protobuf timestamp,
// matching how the scanner stores every other time so stored records compare
// across hosts.
func timespecToTimestamp(timespec syscall.Timespec) *timestamppb.Timestamp {
	return timestamppb.New(time.Unix(timespec.Sec, timespec.Nsec).UTC())
}
