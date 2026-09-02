package appconfig

import (
	"fmt"

	"github.com/google/uuid"
)

// APIKeyGroup identifies which access-level category an API key entry
// belongs to within APIKeysConfig.
type APIKeyGroup string

const (
	APIKeyGroupAdmin    APIKeyGroup = "admin"
	APIKeyGroupUser     APIKeyGroup = "user"
	APIKeyGroupWebhook  APIKeyGroup = "webhook"
	APIKeyGroupReadOnly APIKeyGroup = "read_only"
)

// AllAPIKeyGroups is the four groups in a fixed order, for callers that scan
// every group (an id-addressed lookup, an id backfill).
var AllAPIKeyGroups = []APIKeyGroup{APIKeyGroupAdmin, APIKeyGroupUser, APIKeyGroupWebhook, APIKeyGroupReadOnly}

// ParseAPIKeyGroup validates s as one of the four known group names,
// rejecting anything else — the wire vocabulary a scoped API key operation
// accepts.
func ParseAPIKeyGroup(s string) (APIKeyGroup, error) {
	switch APIKeyGroup(s) {
	case APIKeyGroupAdmin, APIKeyGroupUser, APIKeyGroupWebhook, APIKeyGroupReadOnly:
		return APIKeyGroup(s), nil
	default:
		return "", fmt.Errorf("unknown API key group %q", s)
	}
}

// FindAPIKeyByID locates the entry with the given id across every group —
// the minted id is unique across the whole table, so an id-addressed
// GetApiKey / UpdateApiKey / DeleteApiKey needs no group. It returns the
// group the entry lives in and its index within that group, or ("", -1) if
// no group holds it.
func FindAPIKeyByID(keys *APIKeysConfig, id string) (APIKeyGroup, int) {
	if id == "" {
		return "", -1
	}
	for _, group := range AllAPIKeyGroups {
		if index := FindAPIKeyIndex(keys, group, id); index != -1 {
			return group, index
		}
	}
	return "", -1
}

// APIKeyEntriesFor returns the entries held in group, or nil if group names
// an unknown group.
func APIKeyEntriesFor(keys *APIKeysConfig, group APIKeyGroup) []*APIKeyEntry {
	switch group {
	case APIKeyGroupAdmin:
		return keys.Admin
	case APIKeyGroupUser:
		return keys.User
	case APIKeyGroupWebhook:
		return keys.Webhook
	case APIKeyGroupReadOnly:
		return keys.ReadOnly
	default:
		return nil
	}
}

// setAPIKeyEntriesFor replaces the entries held in group, ignoring an
// unknown group.
func setAPIKeyEntriesFor(keys *APIKeysConfig, group APIKeyGroup, entries []*APIKeyEntry) {
	switch group {
	case APIKeyGroupAdmin:
		keys.Admin = entries
	case APIKeyGroupUser:
		keys.User = entries
	case APIKeyGroupWebhook:
		keys.Webhook = entries
	case APIKeyGroupReadOnly:
		keys.ReadOnly = entries
	}
}

// FindAPIKeyIndex returns the index of the entry with the given id within
// group, or -1 if group has no such entry or names an unknown group. Keys
// are addressed by this minted id rather than by name, which is optional
// and not unique.
func FindAPIKeyIndex(keys *APIKeysConfig, group APIKeyGroup, id string) int {
	for i, entry := range APIKeyEntriesFor(keys, group) {
		if entry.Id == id {
			return i
		}
	}
	return -1
}

// UpsertAPIKey replaces the entry in group whose id matches entry.Id, or
// appends entry if no id matches. entry.Id must already be set: minting a
// fresh id for a newly created key is the caller's job, at the point a new
// key is actually requested, not this function's.
func UpsertAPIKey(keys *APIKeysConfig, group APIKeyGroup, entry *APIKeyEntry) {
	entries := APIKeyEntriesFor(keys, group)
	if index := FindAPIKeyIndex(keys, group, entry.Id); index >= 0 {
		entries[index] = entry
	} else {
		entries = append(entries, entry)
	}
	setAPIKeyEntriesFor(keys, group, entries)
}

// DeleteAPIKey removes the entry matching id from group, reporting whether
// one was found and removed.
func DeleteAPIKey(keys *APIKeysConfig, group APIKeyGroup, id string) bool {
	index := FindAPIKeyIndex(keys, group, id)
	if index == -1 {
		return false
	}
	entries := APIKeyEntriesFor(keys, group)
	setAPIKeyEntriesFor(keys, group, append(entries[:index], entries[index+1:]...))
	return true
}

// BackfillAPIKeyIDs mints an id for every entry, across all four groups,
// that does not already have one — entries stored before this field
// existed — and reports how many were minted. Idempotent: a table where
// every entry already has an id is left untouched and returns 0.
func BackfillAPIKeyIDs(keys *APIKeysConfig) int {
	minted := 0
	for _, group := range []APIKeyGroup{APIKeyGroupAdmin, APIKeyGroupUser, APIKeyGroupWebhook, APIKeyGroupReadOnly} {
		for _, entry := range APIKeyEntriesFor(keys, group) {
			if entry.Id == "" {
				entry.Id = uuid.NewString()
				minted++
			}
		}
	}
	return minted
}
