package aip

import (
	"errors"
	"fmt"
	"strings"

	"Metarr/internal/shared/appconfig"
)

// ErrMalformedName is the sentinel every resource-name / parent parse helper
// wraps when its input does not match the collection's pattern. The service
// layer maps it to connect.CodeInvalidArgument.
var ErrMalformedName = errors.New("malformed resource name")

// Collection segments for the five config resource-name patterns. Each is
// the plural, lowerCamelCase resource noun AIP-122 asks for.
const (
	CollectionAgents          = "agents"
	CollectionSonarrInstances = "sonarrInstances"
	CollectionScanDirectories = "scanDirectories"
	CollectionSidecarTypes    = "sidecarTypes"
	CollectionAccessLevels    = "accessLevels"
	CollectionAPIKeys         = "apiKeys"
)

// AgentName formats the resource name for an agent addressed by slug.
func AgentName(slug string) string { return formatSingle(CollectionAgents, slug) }

// ParseAgentName returns the slug in an `agents/{slug}` resource name.
func ParseAgentName(name string) (slug string, err error) {
	return parseSingle(name, CollectionAgents)
}

// SonarrInstanceName formats the resource name for a Sonarr instance
// addressed by slug.
func SonarrInstanceName(slug string) string { return formatSingle(CollectionSonarrInstances, slug) }

// ParseSonarrInstanceName returns the slug in a `sonarrInstances/{slug}`
// resource name.
func ParseSonarrInstanceName(name string) (slug string, err error) {
	return parseSingle(name, CollectionSonarrInstances)
}

// ScanDirectoryName formats the resource name for a scan directory addressed
// by its scanner slug.
func ScanDirectoryName(slug string) string { return formatSingle(CollectionScanDirectories, slug) }

// ParseScanDirectoryName returns the scanner slug in a
// `scanDirectories/{slug}` resource name.
func ParseScanDirectoryName(name string) (slug string, err error) {
	return parseSingle(name, CollectionScanDirectories)
}

// SidecarTypeName formats the resource name for a sidecar type addressed by
// its minted id.
func SidecarTypeName(id string) string { return formatSingle(CollectionSidecarTypes, id) }

// ParseSidecarTypeName returns the id in a `sidecarTypes/{id}` resource name.
func ParseSidecarTypeName(name string) (id string, err error) {
	return parseSingle(name, CollectionSidecarTypes)
}

// AccessLevelName formats the un-serviced parent path an API key nests
// under. level must be one of the fixed four access levels.
func AccessLevelName(level appconfig.APIKeyGroup) string {
	return CollectionAccessLevels + "/" + string(level)
}

// APIKeyName formats the resource name for an API key: it nests under its
// access level, `accessLevels/{level}/apiKeys/{id}`.
func APIKeyName(level appconfig.APIKeyGroup, id string) string {
	return AccessLevelName(level) + "/" + CollectionAPIKeys + "/" + id
}

// ParseAPIKeyParent returns the access level named by an
// `accessLevels/{level}` parent, validated against the fixed four.
func ParseAPIKeyParent(parent string) (appconfig.APIKeyGroup, error) {
	parts := strings.Split(parent, "/")
	if len(parts) != 2 || parts[0] != CollectionAccessLevels || parts[1] == "" {
		return "", fmt.Errorf("%w: %q is not %s/{level}", ErrMalformedName, parent, CollectionAccessLevels)
	}
	return parseLevel(parts[1])
}

// ParseAPIKeyName returns the access level and id in an
// `accessLevels/{level}/apiKeys/{id}` resource name. The level is validated
// against the fixed four.
func ParseAPIKeyName(name string) (level appconfig.APIKeyGroup, id string, err error) {
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != CollectionAccessLevels || parts[2] != CollectionAPIKeys ||
		parts[1] == "" || parts[3] == "" {
		return "", "", fmt.Errorf("%w: %q is not %s/{level}/%s/{id}",
			ErrMalformedName, name, CollectionAccessLevels, CollectionAPIKeys)
	}
	level, err = parseLevel(parts[1])
	if err != nil {
		return "", "", err
	}
	return level, parts[3], nil
}

// formatSingle joins a collection and id into a two-segment resource name —
// the format counterpart of parseSingle.
func formatSingle(collection, id string) string {
	return collection + "/" + id
}

// parseSingle pulls {id} out of a two-segment `{collection}/{id}` name,
// rejecting a wrong collection, a missing id, or an extra segment.
func parseSingle(name, collection string) (string, error) {
	parts := strings.Split(name, "/")
	if len(parts) != 2 || parts[0] != collection || parts[1] == "" {
		return "", fmt.Errorf("%w: %q is not %s/{id}", ErrMalformedName, name, collection)
	}
	return parts[1], nil
}

// parseLevel validates one access-level segment against appconfig's fixed
// four, re-wrapping the rejection as ErrMalformedName so the whole
// name-parsing surface reports one sentinel.
func parseLevel(segment string) (appconfig.APIKeyGroup, error) {
	level, err := appconfig.ParseAPIKeyGroup(segment)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrMalformedName, err)
	}
	return level, nil
}
