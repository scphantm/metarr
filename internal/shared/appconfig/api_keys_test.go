package appconfig

import "testing"

func TestFindAPIKeyIndex_FindsAMatchingID(t *testing.T) {
	keys := &APIKeysConfig{
		Admin: []*APIKeyEntry{
			{Id: "a", Name: "first"},
			{Id: "b", Name: "second"},
		},
	}

	if index := FindAPIKeyIndex(keys, APIKeyGroupAdmin, "b"); index != 1 {
		t.Fatalf("index = %d, want 1", index)
	}
}

func TestFindAPIKeyIndex_ReportsMinusOneForAnUnknownID(t *testing.T) {
	keys := &APIKeysConfig{Admin: []*APIKeyEntry{{Id: "a"}}}

	if index := FindAPIKeyIndex(keys, APIKeyGroupAdmin, "unknown"); index != -1 {
		t.Fatalf("index = %d, want -1", index)
	}
}

func TestFindAPIKeyIndex_ReportsMinusOneForAnUnknownGroup(t *testing.T) {
	keys := &APIKeysConfig{Admin: []*APIKeyEntry{{Id: "a"}}}

	if index := FindAPIKeyIndex(keys, APIKeyGroup("bogus"), "a"); index != -1 {
		t.Fatalf("index = %d, want -1", index)
	}
}

func TestUpsertAPIKey_ReplacesAMatchingEntry(t *testing.T) {
	keys := &APIKeysConfig{
		Admin: []*APIKeyEntry{
			{Id: "a", Name: "old-name", ApiKey: "old-key"},
			{Id: "b", Name: "sibling"},
		},
	}

	UpsertAPIKey(keys, APIKeyGroupAdmin, &APIKeyEntry{Id: "a", Name: "new-name", ApiKey: "new-key"})

	if len(keys.Admin) != 2 {
		t.Fatalf("expected the group to stay at 2 entries, got %d", len(keys.Admin))
	}
	if keys.Admin[0].Name != "new-name" || keys.Admin[0].ApiKey != "new-key" {
		t.Fatalf("entry was not replaced: %+v", keys.Admin[0])
	}
	if keys.Admin[1].Name != "sibling" {
		t.Fatalf("sibling entry was disturbed: %+v", keys.Admin[1])
	}
}

func TestUpsertAPIKey_AppendsAnUnknownID(t *testing.T) {
	keys := &APIKeysConfig{Admin: []*APIKeyEntry{{Id: "a", Name: "existing"}}}

	UpsertAPIKey(keys, APIKeyGroupAdmin, &APIKeyEntry{Id: "new-id", Name: "new-entry"})

	if len(keys.Admin) != 2 {
		t.Fatalf("expected 2 entries after appending, got %d", len(keys.Admin))
	}
	if keys.Admin[1].Id != "new-id" || keys.Admin[1].Name != "new-entry" {
		t.Fatalf("new entry not appended correctly: %+v", keys.Admin[1])
	}
}

func TestUpsertAPIKey_LeavesOtherGroupsUntouched(t *testing.T) {
	keys := &APIKeysConfig{
		Admin: []*APIKeyEntry{{Id: "a", Name: "admin-key"}},
		User:  []*APIKeyEntry{{Id: "u", Name: "user-key"}},
	}

	UpsertAPIKey(keys, APIKeyGroupAdmin, &APIKeyEntry{Id: "a", Name: "renamed"})

	if len(keys.User) != 1 || keys.User[0].Name != "user-key" {
		t.Fatalf("user group was disturbed: %+v", keys.User)
	}
}

func TestDeleteAPIKey_RemovesExactlyOneEntry(t *testing.T) {
	keys := &APIKeysConfig{
		Admin: []*APIKeyEntry{
			{Id: "a", Name: "keep-1"},
			{Id: "b", Name: "remove-me"},
			{Id: "c", Name: "keep-2"},
		},
	}

	removed := DeleteAPIKey(keys, APIKeyGroupAdmin, "b")
	if !removed {
		t.Fatal("expected DeleteAPIKey to report the entry was found")
	}
	if len(keys.Admin) != 2 {
		t.Fatalf("expected 2 entries remaining, got %d", len(keys.Admin))
	}
	for _, entry := range keys.Admin {
		if entry.Id == "b" {
			t.Fatal("deleted entry is still present")
		}
	}
}

func TestDeleteAPIKey_ReportsFalseForAnUnknownID(t *testing.T) {
	keys := &APIKeysConfig{Admin: []*APIKeyEntry{{Id: "a"}}}

	removed := DeleteAPIKey(keys, APIKeyGroupAdmin, "unknown")
	if removed {
		t.Fatal("expected DeleteAPIKey to report false for an unknown id")
	}
	if len(keys.Admin) != 1 {
		t.Fatalf("group should be untouched, got %d entries", len(keys.Admin))
	}
}

func TestAPIKeyEntries_SharingANameStayIndependentlyAddressable(t *testing.T) {
	keys := &APIKeysConfig{
		Admin: []*APIKeyEntry{
			{Id: "a", Name: "duplicate"},
			{Id: "b", Name: "duplicate"},
		},
	}

	UpsertAPIKey(keys, APIKeyGroupAdmin, &APIKeyEntry{Id: "a", Name: "duplicate", ApiKey: "changed"})
	removed := DeleteAPIKey(keys, APIKeyGroupAdmin, "b")

	if !removed {
		t.Fatal("expected the second same-named entry to still be addressable by id")
	}
	if len(keys.Admin) != 1 || keys.Admin[0].Id != "a" || keys.Admin[0].ApiKey != "changed" {
		t.Fatalf("unexpected state: %+v", keys.Admin)
	}
}

func TestAPIKeyEntry_RenamingLeavesItAddressableByTheSameID(t *testing.T) {
	keys := &APIKeysConfig{Admin: []*APIKeyEntry{{Id: "a", Name: "original"}}}

	UpsertAPIKey(keys, APIKeyGroupAdmin, &APIKeyEntry{Id: "a", Name: "renamed"})

	if index := FindAPIKeyIndex(keys, APIKeyGroupAdmin, "a"); index == -1 {
		t.Fatal("renamed entry is no longer addressable by its id")
	}
	removed := DeleteAPIKey(keys, APIKeyGroupAdmin, "a")
	if !removed {
		t.Fatal("renamed entry could not be deleted by its original id")
	}
}

func TestBackfillAPIKeyIDs_MintsOnlyMissingIDs(t *testing.T) {
	keys := &APIKeysConfig{
		Admin:   []*APIKeyEntry{{Name: "already-has-id", Id: "existing"}, {Name: "needs-one"}},
		User:    []*APIKeyEntry{{Name: "also-needs-one"}},
		Webhook: []*APIKeyEntry{{Name: "keeps-id", Id: "existing-2"}},
	}

	minted := BackfillAPIKeyIDs(keys)
	if minted != 2 {
		t.Fatalf("minted = %d, want 2", minted)
	}

	if keys.Admin[0].Id != "existing" {
		t.Fatalf("existing id was overwritten: %+v", keys.Admin[0])
	}
	if keys.Admin[1].Id == "" {
		t.Fatal("expected an id to be minted for the second admin entry")
	}
	if keys.User[0].Id == "" {
		t.Fatal("expected an id to be minted for the user entry")
	}
	if keys.Webhook[0].Id != "existing-2" {
		t.Fatalf("existing webhook id was overwritten: %+v", keys.Webhook[0])
	}
}

func TestBackfillAPIKeyIDs_IsIdempotent(t *testing.T) {
	keys := &APIKeysConfig{Admin: []*APIKeyEntry{{Name: "needs-one"}}}

	firstRun := BackfillAPIKeyIDs(keys)
	if firstRun != 1 {
		t.Fatalf("first run minted = %d, want 1", firstRun)
	}
	mintedID := keys.Admin[0].Id

	secondRun := BackfillAPIKeyIDs(keys)
	if secondRun != 0 {
		t.Fatalf("second run minted = %d, want 0", secondRun)
	}
	if keys.Admin[0].Id != mintedID {
		t.Fatalf("id changed across runs: %q -> %q", mintedID, keys.Admin[0].Id)
	}
}

func TestBackfillAPIKeyIDs_EmptyTableMintsNothing(t *testing.T) {
	keys := &APIKeysConfig{}

	if minted := BackfillAPIKeyIDs(keys); minted != 0 {
		t.Fatalf("minted = %d, want 0", minted)
	}
}
