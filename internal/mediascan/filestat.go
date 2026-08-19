package mediascan

// This file carries the filesystem's own description of a scanned file — what
// stat(2) reports, as opposed to what the naming conventions imply. The record
// type and the owner-name cache live here; the platform-specific extraction
// lives in filestat_darwin.go, filestat_linux.go and filestat_other.go, which
// all supply fileStatFrom and statSupportsOwnerIDs.

import (
	"io/fs"
	"os/user"
	"strconv"
	"time"
)

// FileStat is the filesystem's own description of a file, captured at scan
// time. It is stored as a pointer on the records that carry it, so a platform
// that cannot supply one records nothing rather than a block of zeros.
type FileStat struct {
	// Mode is the symbolic rendering, "-rw-r--r--", and ModeBits the numeric
	// permission bits alongside setuid, setgid and sticky — which is why the
	// raw stat mode is masked rather than read through fs.FileMode.Perm, since
	// that drops the three special bits.
	Mode     string `bson:"mode" json:"mode"`
	ModeBits uint32 `bson:"mode_bits" json:"mode_bits"`

	UID       uint32 `bson:"uid" json:"uid"`
	GID       uint32 `bson:"gid" json:"gid"`
	OwnerName string `bson:"owner_name,omitempty" json:"owner_name,omitempty"`
	GroupName string `bson:"group_name,omitempty" json:"group_name,omitempty"`

	Inode     uint64 `bson:"inode" json:"inode"`
	LinkCount uint64 `bson:"link_count" json:"link_count"`
	DeviceID  uint64 `bson:"device_id" json:"device_id"`

	SizeBytes int64 `bson:"size_bytes" json:"size_bytes"`
	BlockSize int64 `bson:"block_size" json:"block_size"`
	Blocks    int64 `bson:"blocks" json:"blocks"`

	AccessedAt time.Time `bson:"accessed_at" json:"accessed_at"`
	ModifiedAt time.Time `bson:"modified_at" json:"modified_at"`
	ChangedAt  time.Time `bson:"changed_at" json:"changed_at"`
	// CreatedAt is the file's birth time. It stays zero on Linux, which does
	// not expose one through stat(2).
	CreatedAt time.Time `bson:"created_at,omitempty" json:"created_at,omitempty"`

	// IsSymlink reports that this record describes a symbolic link rather than
	// the file it points at. The scan walks with lstat, so a symlinked sidecar
	// reports the link's own stat and says so here instead of quietly reporting
	// its target's.
	IsSymlink bool `bson:"is_symlink" json:"is_symlink"`
}

// portableFileStat fills the fields available from fs.FileInfo alone. Every
// platform starts here and then layers on whatever its stat structure adds.
func portableFileStat(info fs.FileInfo) *FileStat {
	return &FileStat{
		Mode:       info.Mode().String(),
		ModeBits:   uint32(info.Mode().Perm()),
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime().UTC(),
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
func (cache *ownerNameCache) resolve(fileStat *FileStat) {
	if fileStat == nil || !statSupportsOwnerIDs {
		return
	}

	if cache.userNames == nil {
		cache.userNames = map[uint32]string{}
		cache.groupNames = map[uint32]string{}
	}

	name, known := cache.userNames[fileStat.UID]
	if !known {
		// A miss is cached as the empty string too: an id belonging to no local
		// account is common on a mounted library, and re-asking once per file
		// would be the expensive way to get the same answer.
		if account, err := user.LookupId(strconv.FormatUint(uint64(fileStat.UID), 10)); err == nil {
			name = account.Username
		}
		cache.userNames[fileStat.UID] = name
	}
	fileStat.OwnerName = name

	name, known = cache.groupNames[fileStat.GID]
	if !known {
		if group, err := user.LookupGroupId(strconv.FormatUint(uint64(fileStat.GID), 10)); err == nil {
			name = group.Name
		}
		cache.groupNames[fileStat.GID] = name
	}
	fileStat.GroupName = name
}
