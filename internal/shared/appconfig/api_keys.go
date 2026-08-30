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

func (k APIKeysConfig) entriesFor(group APIKeyGroup) []APIKeyEntry {
	switch group {
	case APIKeyGroupAdmin:
		return k.Admin
	case APIKeyGroupUser:
		return k.User
	case APIKeyGroupWebhook:
		return k.Webhook
	case APIKeyGroupReadOnly:
		return k.ReadOnly
	default:
		return nil
	}
}

func (k *APIKeysConfig) setEntriesFor(group APIKeyGroup, entries []APIKeyEntry) {
	switch group {
	case APIKeyGroupAdmin:
		k.Admin = entries
	case APIKeyGroupUser:
		k.User = entries
	case APIKeyGroupWebhook:
		k.Webhook = entries
	case APIKeyGroupReadOnly:
		k.ReadOnly = entries
	}
}

// FindAPIKeyIndex returns the index of the entry with the given id within
// group, or -1 if group has no such entry or names an unknown group. Keys
// are addressed by this minted id rather than by name, which is optional
// and not unique.
func (k APIKeysConfig) FindAPIKeyIndex(group APIKeyGroup, id string) int {
	for i, entry := range k.entriesFor(group) {
		if entry.ID == id {
			return i
		}
	}
	return -1
}

// UpsertAPIKey replaces the entry in group whose id matches entry.ID, or
// appends entry if no id matches. entry.ID must already be set: minting a
// fresh id for a newly created key is the caller's job, at the point a new
// key is actually requested, not this function's.
func (k *APIKeysConfig) UpsertAPIKey(group APIKeyGroup, entry APIKeyEntry) {
	entries := k.entriesFor(group)
	if index := k.FindAPIKeyIndex(group, entry.ID); index >= 0 {
		entries[index] = entry
	} else {
		entries = append(entries, entry)
	}
	k.setEntriesFor(group, entries)
}

// DeleteAPIKey removes the entry matching id from group, reporting whether
// one was found and removed.
func (k *APIKeysConfig) DeleteAPIKey(group APIKeyGroup, id string) bool {
	index := k.FindAPIKeyIndex(group, id)
	if index == -1 {
		return false
	}
	entries := k.entriesFor(group)
	k.setEntriesFor(group, append(entries[:index], entries[index+1:]...))
	return true
}

// BackfillAPIKeyIDs mints an id for every entry, across all four groups,
// that does not already have one — entries stored before this field
// existed — and reports how many were minted. Idempotent: a table where
// every entry already has an id is left untouched and returns 0.
func BackfillAPIKeyIDs(keys *APIKeysConfig) int {
	minted := 0
	for _, group := range []APIKeyGroup{APIKeyGroupAdmin, APIKeyGroupUser, APIKeyGroupWebhook, APIKeyGroupReadOnly} {
		entries := keys.entriesFor(group)
		for i := range entries {
			if entries[i].ID == "" {
				entries[i].ID = uuid.NewString()
				minted++
			}
		}
		keys.setEntriesFor(group, entries)
	}
	return minted
}
