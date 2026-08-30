package appconfig

import "testing"

func TestFindAPIKeyIndex_FindsAMatchingID(t *testing.T) {
	keys := APIKeysConfig{
		Admin: []APIKeyEntry{
			{ID: "a", Name: "first"},
			{ID: "b", Name: "second"},
		},
	}

	if index := keys.FindAPIKeyIndex(APIKeyGroupAdmin, "b"); index != 1 {
		t.Fatalf("index = %d, want 1", index)
	}
}

func TestFindAPIKeyIndex_ReportsMinusOneForAnUnknownID(t *testing.T) {
	keys := APIKeysConfig{Admin: []APIKeyEntry{{ID: "a"}}}

	if index := keys.FindAPIKeyIndex(APIKeyGroupAdmin, "unknown"); index != -1 {
		t.Fatalf("index = %d, want -1", index)
	}
}

func TestFindAPIKeyIndex_ReportsMinusOneForAnUnknownGroup(t *testing.T) {
	keys := APIKeysConfig{Admin: []APIKeyEntry{{ID: "a"}}}

	if index := keys.FindAPIKeyIndex(APIKeyGroup("bogus"), "a"); index != -1 {
		t.Fatalf("index = %d, want -1", index)
	}
}

func TestUpsertAPIKey_ReplacesAMatchingEntry(t *testing.T) {
	keys := APIKeysConfig{
		Admin: []APIKeyEntry{
			{ID: "a", Name: "old-name", Key: "old-key"},
			{ID: "b", Name: "sibling"},
		},
	}

	keys.UpsertAPIKey(APIKeyGroupAdmin, APIKeyEntry{ID: "a", Name: "new-name", Key: "new-key"})

	if len(keys.Admin) != 2 {
		t.Fatalf("expected the group to stay at 2 entries, got %d", len(keys.Admin))
	}
	if keys.Admin[0].Name != "new-name" || keys.Admin[0].Key != "new-key" {
		t.Fatalf("entry was not replaced: %+v", keys.Admin[0])
	}
	if keys.Admin[1].Name != "sibling" {
		t.Fatalf("sibling entry was disturbed: %+v", keys.Admin[1])
	}
}

func TestUpsertAPIKey_AppendsAnUnknownID(t *testing.T) {
	keys := APIKeysConfig{Admin: []APIKeyEntry{{ID: "a", Name: "existing"}}}

	keys.UpsertAPIKey(APIKeyGroupAdmin, APIKeyEntry{ID: "new-id", Name: "new-entry"})

	if len(keys.Admin) != 2 {
		t.Fatalf("expected 2 entries after appending, got %d", len(keys.Admin))
	}
	if keys.Admin[1].ID != "new-id" || keys.Admin[1].Name != "new-entry" {
		t.Fatalf("new entry not appended correctly: %+v", keys.Admin[1])
	}
}

func TestUpsertAPIKey_LeavesOtherGroupsUntouched(t *testing.T) {
	keys := APIKeysConfig{
		Admin: []APIKeyEntry{{ID: "a", Name: "admin-key"}},
		User:  []APIKeyEntry{{ID: "u", Name: "user-key"}},
	}

	keys.UpsertAPIKey(APIKeyGroupAdmin, APIKeyEntry{ID: "a", Name: "renamed"})

	if len(keys.User) != 1 || keys.User[0].Name != "user-key" {
		t.Fatalf("user group was disturbed: %+v", keys.User)
	}
}

func TestDeleteAPIKey_RemovesExactlyOneEntry(t *testing.T) {
	keys := APIKeysConfig{
		Admin: []APIKeyEntry{
			{ID: "a", Name: "keep-1"},
			{ID: "b", Name: "remove-me"},
			{ID: "c", Name: "keep-2"},
		},
	}

	removed := keys.DeleteAPIKey(APIKeyGroupAdmin, "b")
	if !removed {
		t.Fatal("expected DeleteAPIKey to report the entry was found")
	}
	if len(keys.Admin) != 2 {
		t.Fatalf("expected 2 entries remaining, got %d", len(keys.Admin))
	}
	for _, entry := range keys.Admin {
		if entry.ID == "b" {
			t.Fatal("deleted entry is still present")
		}
	}
}

func TestDeleteAPIKey_ReportsFalseForAnUnknownID(t *testing.T) {
	keys := APIKeysConfig{Admin: []APIKeyEntry{{ID: "a"}}}

	removed := keys.DeleteAPIKey(APIKeyGroupAdmin, "unknown")
	if removed {
		t.Fatal("expected DeleteAPIKey to report false for an unknown id")
	}
	if len(keys.Admin) != 1 {
		t.Fatalf("group should be untouched, got %d entries", len(keys.Admin))
	}
}

func TestAPIKeyEntries_SharingANameStayIndependentlyAddressable(t *testing.T) {
	keys := APIKeysConfig{
		Admin: []APIKeyEntry{
			{ID: "a", Name: "duplicate"},
			{ID: "b", Name: "duplicate"},
		},
	}

	keys.UpsertAPIKey(APIKeyGroupAdmin, APIKeyEntry{ID: "a", Name: "duplicate", Key: "changed"})
	removed := keys.DeleteAPIKey(APIKeyGroupAdmin, "b")

	if !removed {
		t.Fatal("expected the second same-named entry to still be addressable by id")
	}
	if len(keys.Admin) != 1 || keys.Admin[0].ID != "a" || keys.Admin[0].Key != "changed" {
		t.Fatalf("unexpected state: %+v", keys.Admin)
	}
}

func TestAPIKeyEntry_RenamingLeavesItAddressableByTheSameID(t *testing.T) {
	keys := APIKeysConfig{Admin: []APIKeyEntry{{ID: "a", Name: "original"}}}

	keys.UpsertAPIKey(APIKeyGroupAdmin, APIKeyEntry{ID: "a", Name: "renamed"})

	if index := keys.FindAPIKeyIndex(APIKeyGroupAdmin, "a"); index == -1 {
		t.Fatal("renamed entry is no longer addressable by its id")
	}
	removed := keys.DeleteAPIKey(APIKeyGroupAdmin, "a")
	if !removed {
		t.Fatal("renamed entry could not be deleted by its original id")
	}
}

func TestBackfillAPIKeyIDs_MintsOnlyMissingIDs(t *testing.T) {
	keys := APIKeysConfig{
		Admin:   []APIKeyEntry{{Name: "already-has-id", ID: "existing"}, {Name: "needs-one"}},
		User:    []APIKeyEntry{{Name: "also-needs-one"}},
		Webhook: []APIKeyEntry{{Name: "keeps-id", ID: "existing-2"}},
	}

	minted := BackfillAPIKeyIDs(&keys)
	if minted != 2 {
		t.Fatalf("minted = %d, want 2", minted)
	}

	if keys.Admin[0].ID != "existing" {
		t.Fatalf("existing id was overwritten: %+v", keys.Admin[0])
	}
	if keys.Admin[1].ID == "" {
		t.Fatal("expected an id to be minted for the second admin entry")
	}
	if keys.User[0].ID == "" {
		t.Fatal("expected an id to be minted for the user entry")
	}
	if keys.Webhook[0].ID != "existing-2" {
		t.Fatalf("existing webhook id was overwritten: %+v", keys.Webhook[0])
	}
}

func TestBackfillAPIKeyIDs_IsIdempotent(t *testing.T) {
	keys := APIKeysConfig{Admin: []APIKeyEntry{{Name: "needs-one"}}}

	firstRun := BackfillAPIKeyIDs(&keys)
	if firstRun != 1 {
		t.Fatalf("first run minted = %d, want 1", firstRun)
	}
	mintedID := keys.Admin[0].ID

	secondRun := BackfillAPIKeyIDs(&keys)
	if secondRun != 0 {
		t.Fatalf("second run minted = %d, want 0", secondRun)
	}
	if keys.Admin[0].ID != mintedID {
		t.Fatalf("id changed across runs: %q -> %q", mintedID, keys.Admin[0].ID)
	}
}

func TestBackfillAPIKeyIDs_EmptyTableMintsNothing(t *testing.T) {
	keys := APIKeysConfig{}

	if minted := BackfillAPIKeyIDs(&keys); minted != 0 {
		t.Fatalf("minted = %d, want 0", minted)
	}
}
