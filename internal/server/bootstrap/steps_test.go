package bootstrap

import (
	"encoding/json"
	"testing"
)

func TestResolveGUIDsJSON_EveryOccurrenceGetsAnIndependentValue(t *testing.T) {
	raw := []byte(`{
		"admin":     {"id": "{guid}", "name": "Administrator Key", "api_key": "{guid}"},
		"user":      {"id": "{guid}", "name": "User Key",          "api_key": "{guid}"},
		"webhook":   {"id": "{guid}", "name": "Webhook Key",       "api_key": "{guid}"},
		"read_only": {"id": "{guid}", "name": "Read Only Key",     "api_key": "{guid}"}
	}`)

	resolved, err := resolveGUIDsJSON(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]map[string]string
	if err := json.Unmarshal(resolved, &parsed); err != nil {
		t.Fatalf("resolved output did not parse: %v", err)
	}

	seen := make(map[string]bool)
	count := 0
	for group, entry := range parsed {
		for field, value := range entry {
			if field != "id" && field != "api_key" {
				continue
			}
			count++
			if seen[value] {
				t.Fatalf("%s.%s reused a value already seen elsewhere: %q — every {guid} occurrence must be independent", group, field, value)
			}
			seen[value] = true
		}
	}
	if count != 8 {
		t.Fatalf("expected 8 substituted fields (4 groups x id+api_key), found %d", count)
	}
}

func TestResolveGUIDsJSON_OnlyAnExactWholeValueMatchIsSubstituted(t *testing.T) {
	raw := []byte(`{"note": "prefix-{guid}-suffix", "id": "{guid}"}`)

	resolved, err := resolveGUIDsJSON(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Note string `json:"note"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(resolved, &parsed); err != nil {
		t.Fatalf("resolved output did not parse: %v", err)
	}

	if parsed.Note != "prefix-{guid}-suffix" {
		t.Errorf("note = %q, want unchanged: only a whole-value match should ever be substituted", parsed.Note)
	}
	if parsed.ID == "{guid}" || parsed.ID == "" {
		t.Errorf("id = %q, want a substituted value", parsed.ID)
	}
}
