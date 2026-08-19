package scanmodel

// This file carries the filesystem's own description of a scanned file — what
// stat(2) reports, as opposed to what the naming conventions imply. Only the
// record lives here; the code that produces one has to run on the machine
// holding the file, so it lives in the agent's mediascan package.

import "time"

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
