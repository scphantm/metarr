package mediascan

// This file produces a scanmodel.FileStat from what the filesystem reports.
// The platform-specific extraction lives in filestat_darwin.go,
// filestat_linux.go and filestat_other.go, which all supply fileStatFrom and
// statSupportsOwnerIDs.

import (
	"io/fs"
	"os/user"
	"strconv"

	"google.golang.org/protobuf/types/known/timestamppb"

	"Metarr/internal/shared/scanmodel"
)

// portableFileStat fills the fields available from fs.FileInfo alone. Every
// platform starts here and then layers on whatever its stat structure adds.
func portableFileStat(info fs.FileInfo) *scanmodel.FileStat {
	return &scanmodel.FileStat{
		Mode:       info.Mode().String(),
		ModeBits:   uint32(info.Mode().Perm()),
		SizeBytes:  info.Size(),
		ModifiedAt: timestamppb.New(info.ModTime().UTC()),
		IsSymlink:  info.Mode()&fs.ModeSymlink != 0,
	}
}

// ownerNameCache resolves user and group ids to names, remembering what it has
// already looked up. A library of thousands of files holds a handful of distinct
// owners, so this turns thousands of lookups into a few.
//
// The zero value is usable: the maps are created on first use.
type ownerNameCache struct {
	userNames  map[uint32]string
	groupNames map[uint32]string
}

// resolve fills in fileStat's owner and group names in place. A nil stat, or a
// platform whose stat carries no ids at all, is left alone — resolving an id
// that was never supplied would report every file as owned by root.
func (cache *ownerNameCache) resolve(fileStat *scanmodel.FileStat) {
	if fileStat == nil || !statSupportsOwnerIDs {
		return
	}

	if cache.userNames == nil {
		cache.userNames = map[uint32]string{}
		cache.groupNames = map[uint32]string{}
	}

	name, known := cache.userNames[fileStat.Uid]
	if !known {
		// A miss is cached as the empty string too: an id belonging to no local
		// account is common on a mounted library, and re-asking once per file
		// would be the expensive way to get the same answer.
		if account, err := user.LookupId(strconv.FormatUint(uint64(fileStat.Uid), 10)); err == nil {
			name = account.Username
		}
		cache.userNames[fileStat.Uid] = name
	}
	fileStat.OwnerName = name

	name, known = cache.groupNames[fileStat.Gid]
	if !known {
		if group, err := user.LookupGroupId(strconv.FormatUint(uint64(fileStat.Gid), 10)); err == nil {
			name = group.Name
		}
		cache.groupNames[fileStat.Gid] = name
	}
	fileStat.GroupName = name
}
