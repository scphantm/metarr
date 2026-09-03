// Package agentregistry is the server's view of its agents: what they are
// allowed to know, who is currently alive, and how to translate what they
// report back into the server's own terms.
package agentregistry

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/appconfig"
)

// BuildProjection returns everything the named agent is allowed to know, and
// nothing else.
//
// This function is the security boundary of the agent design, so it is written
// as an allow-list and must stay that way. The Config it reads from holds the
// admin password salt and hash, the JWT HMAC signing secret, and every Sonarr
// credential; agents run on machines the server does not control, and
// several of them may be operated by different people. Building the projection
// by copying the config and deleting the secrets would mean every future field
// is exposed by default, and the one someone forgets to delete is a credential
// on someone else's box.
//
// Adding a field here is a decision that every agent host may read it.
func BuildProjection(config *appconfig.Config, slug string, updatedAt time.Time) *agentproto.AgentConfigProjection {
	projection := &agentproto.AgentConfigProjection{
		Slug:          slug,
		ParallelCount: config.DirectoryScanner.ParallelCount,
		SidecarTypes:  config.DirectoryScanner.SidecarTypes,
		Directories:   []*agentproto.MappedDirectory{},
		LogLevel:      appconfig.LogLevelInfo,
		UpdatedAt:     timestamppb.New(updatedAt),
	}

	index := appconfig.FindAgentIndex(config, slug)
	if index < 0 {
		// A connected but unconfigured agent still gets a real log level
		// (defaulted above) rather than an empty string — being able to bump an
		// agent to debug is one of the more useful things to do with one before
		// it has any libraries mapped, e.g. while diagnosing why it isn't
		// configuring. Everything else about it stays idle until it is.
		return projection
	}

	agent := config.Agents[index]
	projection.DisplayName = agent.DisplayName
	if agent.LogLevel != "" {
		projection.LogLevel = agent.LogLevel
	}

	for _, mapping := range agent.Mappings {
		// A mapping naming a scan directory that no longer exists is skipped
		// rather than sent as a half-record: the agent would have no scan type
		// to walk it with.
		scannerIndex := appconfig.FindScanDirectoryIndex(config.DirectoryScanner, mapping.ScannerSlug)
		if scannerIndex < 0 {
			continue
		}
		if mapping.AgentPath == "" {
			// An empty path is how the UI says "this agent cannot see this
			// library", so it is a mapping to leave out, not one to report.
			continue
		}

		projection.Directories = append(projection.Directories, &agentproto.MappedDirectory{
			ScannerSlug: mapping.ScannerSlug,
			ScanType:    config.DirectoryScanner.ScanDirectories[scannerIndex].ScanType,
			AgentPath:   mapping.AgentPath,
		})
	}

	return projection
}
