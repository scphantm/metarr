// Package agentproto is the contract between metarr-server and metarr-agent.
// Both binaries import it, so every key name, stream name and payload shape
// exists once rather than as matching string literals on two sides of a
// network.
//
// Three transports are in play, and which one carries what is a deliberate
// choice rather than an accident:
//
//   - Redis keys carry state that is true right now and worthless later.
//     Presence and telemetry are written by the agent with a TTL, so an agent
//     that dies stops refreshing and simply expires. None of it is ever
//     persisted — there is nothing to clean up after a crash.
//
//   - Streams carry work and results, which must survive a restart on either
//     side. A scan requested while the agent is rebooting has to still happen.
//
//   - Pub/Sub carries notifications and synchronous calls, where a missed
//     message costs latency rather than correctness.
package agentproto

import (
	"fmt"
	"time"

	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/metadata"
	"Metarr/internal/shared/scanmodel"
)

// PresenceTTL is how long a presence or lock key outlives its last refresh.
// It is three heartbeats, so a single missed write — a GC pause, a blocked
// write, a slow network — does not make a healthy agent flicker offline.
const (
	HeartbeatInterval = 5 * time.Second
	PresenceTTL       = 15 * time.Second
)

// Redis key builders. The metarr: prefix keeps agent state distinguishable
// from the session and cache keys sharing this database.
const keyPrefix = "metarr:agent:"

// PresenceKey holds an agent's identity and live telemetry. Written by the
// agent every HeartbeatInterval, expiring after PresenceTTL.
func PresenceKey(slug string) string { return keyPrefix + "presence:" + slug }

// LockKey is how an agent claims its slug. Taken with SET NX so a second
// process configured with the same slug fails loudly at startup instead of
// silently competing for the first one's work.
func LockKey(slug string) string { return keyPrefix + "lock:" + slug }

// ConfigKey holds the redacted configuration projection the server publishes
// for one agent. Unlike the others this has no TTL: it is the agent's
// configuration, and it should survive the agent being off overnight.
func ConfigKey(slug string) string { return keyPrefix + "config:" + slug }

// PresenceKeyPattern matches every presence key, for the server's SCAN.
const PresenceKeyPattern = keyPrefix + "presence:*"

// SlugFromPresenceKey recovers the slug from a key returned by that scan.
func SlugFromPresenceKey(key string) string {
	prefix := keyPrefix + "presence:"
	if len(key) <= len(prefix) {
		return ""
	}
	return key[len(prefix):]
}

// CommandStream is the durable stream one agent reads its work from. It is
// per-agent because filesystems are machine-local: a scan of /mnt/tank only
// means anything on the machine that has it mounted, so the work has to be
// addressed rather than offered to whichever agent is free.
func CommandStream(slug string) string { return "events.agent." + slug + ".commands" }

// CommandGroup is the consumer group for an agent's command stream. One agent
// process is expected per slug, enforced by LockKey.
func CommandGroup(slug string) string { return "agent_" + slug + "_group" }

// CommandStreamPattern matches every agent command stream, so the Redis
// statistics dashboard can discover them without knowing the slugs.
const CommandStreamPattern = "events.agent.*.commands"

// Scan results flow back on one shared stream. They are addressed to the
// server rather than to a particular reader, and every message names the agent
// that produced it.
const (
	ScanResultStream = "events.agent_scan_results"
	ScanResultGroup  = "agent_scan_results_group"
)

// Event names carried in the eventbus.Event envelope.
const (
	ScanCommandEventName  = "agent.scan"
	ScanResultEventName   = "agent.scan_result"
	ScanCompleteEventName = "agent.scan_complete"
	ScanFailedEventName   = "agent.scan_failed"
	NFOReadEventName      = "agent.nfo_read"
)

// ConfigChangedChannel tells one agent its configuration has been rewritten
// and it should re-read ConfigKey. Best effort: the agent also re-reads on a
// timer, so a notification lost while it was reconnecting costs a delay rather
// than a stale configuration forever.
func ConfigChangedChannel(slug string) string { return "agent.config.changed." + slug }

// RequestChannel is the agent's Pub/Sub request channel, for calls where an
// HTTP caller is waiting on the answer and a durable stream would be the wrong
// shape. Replies go to the correlation-scoped channel, exactly as the
// heartbeat does.
func RequestChannel(slug string) string { return "agent." + slug + ".request" }

// AgentIdentity is who and where an agent is. Collected once at startup, since
// none of it changes while the process lives.
type AgentIdentity struct {
	Slug string `json:"slug"`
	// InstanceID distinguishes one run of an agent from the next. It is what
	// makes a restart visible as a restart rather than as continuous uptime.
	InstanceID string `json:"instance_id"`

	Hostname string `json:"hostname"`
	IP       string `json:"ip"`

	// UID and Username are the account the service runs as — the thing that
	// decides whether it can actually read the library it was pointed at.
	UID      int    `json:"uid"`
	Username string `json:"username"`

	OS      string    `json:"os"`
	Arch    string    `json:"arch"`
	Version string    `json:"version"`
	Started time.Time `json:"started"`
}

// AgentTelemetry is the live host state, refreshed on every heartbeat.
type AgentTelemetry struct {
	CPUPercent float64 `json:"cpu_percent"`

	MemoryUsedBytes  uint64 `json:"memory_used_bytes"`
	MemoryTotalBytes uint64 `json:"memory_total_bytes"`

	// GPUs is empty on a host without one, which is the common case and not a
	// failure. A host whose GPU cannot be queried also reports none rather
	// than failing the heartbeat around it.
	GPUs []GPUTelemetry `json:"gpus,omitempty"`
}

// GPUTelemetry is one GPU's live state.
type GPUTelemetry struct {
	Name             string  `json:"name"`
	UtilizationPct   float64 `json:"utilization_percent"`
	MemoryUsedBytes  uint64  `json:"memory_used_bytes"`
	MemoryTotalBytes uint64  `json:"memory_total_bytes"`
}

// AgentPresence is the whole value stored at PresenceKey.
type AgentPresence struct {
	Identity   AgentIdentity  `json:"identity"`
	Telemetry  AgentTelemetry `json:"telemetry"`
	ReportedAt time.Time      `json:"reported_at"`
}

// AgentConfigProjection is everything an agent is allowed to know.
//
// This type is the security boundary of the whole design. The application
// config it is derived from holds the admin password hash, all four API key
// categories and every Sonarr credential, and it is published whole onto an
// internal stream. An agent runs on a machine the server does not control, so
// it gets this instead — and adding a field here means deciding, deliberately,
// that every agent host may read it.
type AgentConfigProjection struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name,omitempty"`

	// ParallelCount is how many item directories the agent walks at once.
	// int32 because it is carried straight through from the config section
	// rather than re-typed on the way past.
	ParallelCount int32 `json:"parallel_count"`

	// SidecarTypes is the classification table. The agent has to hold it
	// because classification happens where the files are.
	SidecarTypes []*appconfig.SidecarTypeDefinition `json:"sidecar_types"`

	// Directories are only those mapped to this agent. An agent is told
	// nothing about libraries it cannot see.
	Directories []MappedDirectory `json:"directories"`

	// LogLevel is this agent's own verbosity ("info" or "debug"), set from the
	// System > Logging screen. Unlike everything else in this type it isn't a
	// secret-boundary concern — a log level is safe for any agent to hold —
	// but it lives here because it's still per-agent state delivered the same
	// way as everything else the agent needs.
	LogLevel string `json:"log_level"`

	// UpdatedAt lets the agent log which revision it is running on, which is
	// the difference between "the mapping is wrong" and "the agent never got
	// the new mapping".
	UpdatedAt time.Time `json:"updated_at"`
}

// MappedDirectory is one scan directory as this agent sees it.
type MappedDirectory struct {
	ScannerSlug string `json:"scanner_slug"`
	ScanType    string `json:"scan_type"`
	// AgentPath is the path on the agent's machine. The server's own path for
	// the same library is deliberately absent: the agent has no use for it,
	// and translating back is the server's job on the way in.
	AgentPath string `json:"agent_path"`
}

// FindDirectory returns the mapping for scannerSlug.
func (p AgentConfigProjection) FindDirectory(scannerSlug string) (MappedDirectory, bool) {
	for _, directory := range p.Directories {
		if directory.ScannerSlug == scannerSlug {
			return directory, true
		}
	}
	return MappedDirectory{}, false
}

// ScanCommand asks an agent to walk one mapped directory.
type ScanCommand struct {
	// ScanID ties every result message back to the run that produced it, so
	// results from a superseded scan can be recognised and dropped.
	ScanID      string `json:"scan_id"`
	ScannerSlug string `json:"scanner_slug"`
}

// ScanResultMessage carries one scanned item directory back to the server.
// Results are sent per item rather than as one payload at the end: a library
// scan produces far too much to sit in a single stream entry, and streaming
// them means the library fills in progressively instead of all at once when
// the walk finishes.
type ScanResultMessage struct {
	ScanID      string `json:"scan_id"`
	AgentSlug   string `json:"agent_slug"`
	ScannerSlug string `json:"scanner_slug"`

	// ScanRootPath is the agent's own path for the scan root, which is what
	// the server translates back to its canonical path.
	ScanRootPath string                `json:"scan_root_path"`
	Result       *scanmodel.ScanResult `json:"result"`

	// PartIndex and PartCount split one oversized item across several
	// messages. An item small enough to travel whole reports 0 and 1. Parts
	// after the first carry only the additional media files; the directory
	// record travels with part 0.
	PartIndex int `json:"part_index"`
	PartCount int `json:"part_count"`

	ScannedAt time.Time `json:"scanned_at"`
}

// ScanCompleteMessage ends a scan run. It carries the timestamp taken before
// the walk began, which is what lets the server delete records the scan did
// not touch — anything older than this under the same root is gone from disk.
type ScanCompleteMessage struct {
	ScanID       string    `json:"scan_id"`
	AgentSlug    string    `json:"agent_slug"`
	ScannerSlug  string    `json:"scanner_slug"`
	ScanRootPath string    `json:"scan_root_path"`
	ItemCount    int       `json:"item_count"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
}

// ScanFailedMessage reports a scan that could not run at all — an unreadable
// root, an unmapped slug. A scan that fails part way through reports the items
// it managed plus this, rather than pretending to have completed, because a
// completion would trigger the stale sweep and delete the half it never saw.
type ScanFailedMessage struct {
	ScanID      string `json:"scan_id"`
	AgentSlug   string `json:"agent_slug"`
	ScannerSlug string `json:"scanner_slug"`
	Error       string `json:"error"`
}

// NFOReadRequest asks an agent to read one NFO file as it is on disk right
// now, rather than as the last scan recorded it.
type NFOReadRequest struct {
	ScannerSlug string `json:"scanner_slug"`

	// Both paths are relative, and that is the whole point: an absolute path
	// means different things on the two machines, while the part below the
	// scan root is identical on both. The server strips its own root before
	// sending, the agent joins its own root on arrival, and neither has to
	// know the other's filesystem layout.
	//
	// RelativeDirectory is the item directory below the scan root;
	// RelativePath is the file within that directory.
	RelativeDirectory string `json:"relative_directory"`
	RelativePath      string `json:"relative_path"`
}

// NFOReadReply answers NFOReadRequest. Error is set instead of Metadata when
// the file could not be read; NotFound separates "no such file", which is a
// 404 to the waiting HTTP caller, from a genuine failure.
type NFOReadReply struct {
	Metadata *metadata.Metadata `json:"metadata,omitempty"`
	Error    string             `json:"error,omitempty"`
	NotFound bool               `json:"not_found,omitempty"`
}

// ValidateSlug reports whether slug is usable as an agent name. It has to be
// safe to embed in a Redis key and a stream name, so the accepted set is
// deliberately narrow.
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("agent slug cannot be empty")
	}
	if len(slug) > 63 {
		return fmt.Errorf("agent slug %q is longer than 63 characters", slug)
	}
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
		default:
			return fmt.Errorf(
				"agent slug %q contains %q; use lowercase letters, digits, hyphen or underscore",
				slug, r,
			)
		}
	}
	return nil
}
