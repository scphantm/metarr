package appconfig

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The Go field names on these types are spelled to match what the protobuf
// generator produces for the same fields, so that swapping the hand-written
// struct for the generated message is not also a rename of every call site.
// The names that actually reach a stored document or the wire are the struct
// tags, and those are deliberately *not* spelled that way — api_keys stays
// api_keys, not apiKeys.
//
// That split is the whole reason the reshaping was safe, and it is invisible
// in ordinary use: nothing fails at runtime if someone "tidies" a tag to
// match its field. These tests are what fails instead.

// TestConfigJSONKeysAreStable pins the serialized key names for the config
// document. A renamed Go field that drags its tag along with it changes what
// is written to the database and sent over the wire, which no other test in
// the suite would notice.
func TestConfigJSONKeysAreStable(t *testing.T) {
	// Every field is populated, including the omitempty ones, so that no key
	// is absent from the encoding merely because it was left at zero.
	config := Default()
	config.ID = SingletonID
	config.Admin = &AdminUser{
		Username: "admin", Email: "admin@example.com",
		PasswordSalt: "salt", PasswordHash: "hash",
	}
	config.ApiKeys.Admin = []*APIKeyEntry{{Id: "id", Name: "name", ApiKey: "key"}}
	config.Interfaces.Sonarr = []*SonarrInstance{{
		InstanceName: "n", InstanceSlug: "s", SonarrUrl: "u", SonarrApiKey: "k",
		RootDirMap: []*RootDirMapping{{SonarrPath: "a", LocalPath: "b"}},
		Storage:    &StorageConfig{Mode: "cache", Ttl: "1h", MaxCount: 3},
	}}
	config.DirectoryScanner.ScanDirectories = []*ScanDirectory{{
		ScannerSlug: "s", ScanType: "movie", Directory: "/d",
	}}
	config.DirectoryScanner.SidecarTypes = []*SidecarTypeDefinition{{
		Id: "i", Type: "t", Category: "image", Order: 10,
		Patterns: []string{"p"}, Extensions: []string{".jpg"},
	}}
	config.Agents = []*AgentConfig{{
		Slug: "a", DisplayName: "A", LogLevel: LogLevelInfo,
		Mappings: []*AgentDirectoryMapping{{ScannerSlug: "s", AgentPath: "/p"}},
	}}

	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshalling the config: %v", err)
	}

	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decoding the marshalled config: %v", err)
	}

	assertKeys(t, "config", document, []string{
		"admin", "agents", "api_keys", "directory_scanner", "interfaces", "logging",
	})
	assertKeys(t, "config.admin", document["admin"], []string{
		"email", "password_hash", "password_salt", "username",
	})
	assertKeys(t, "config.api_keys", document["api_keys"], []string{
		"admin", "read_only", "user", "webhook",
	})
	assertKeys(t, "config.api_keys.admin[0]", first(t, document, "api_keys", "admin"), []string{
		"api_key", "id", "name",
	})
	assertKeys(t, "config.interfaces", document["interfaces"], []string{"sonarr"})

	sonarr := first(t, document, "interfaces", "sonarr")
	assertKeys(t, "config.interfaces.sonarr[0]", sonarr, []string{
		"instance_name", "instance_slug", "root_dir_map", "sonarr_api_key", "sonarr_url", "storage",
	})
	assertKeys(t, "config.interfaces.sonarr[0].storage", sonarr.(map[string]any)["storage"], []string{
		"max_count", "mode", "ttl",
	})
	assertKeys(t, "config.interfaces.sonarr[0].root_dir_map[0]",
		firstOf(t, sonarr.(map[string]any)["root_dir_map"]), []string{"local_path", "sonarr_path"})

	assertKeys(t, "config.directory_scanner", document["directory_scanner"], []string{
		"parallel_count", "scan_directories", "sidecar_types",
	})
	assertKeys(t, "config.directory_scanner.scan_directories[0]",
		first(t, document, "directory_scanner", "scan_directories"),
		[]string{"directory", "scan_type", "scanner_slug"})
	assertKeys(t, "config.directory_scanner.sidecar_types[0]",
		first(t, document, "directory_scanner", "sidecar_types"),
		[]string{"category", "extensions", "id", "order", "patterns", "type"})

	agent := firstOf(t, document["agents"])
	assertKeys(t, "config.agents[0]", agent, []string{
		"display_name", "log_level", "mappings", "slug",
	})
	assertKeys(t, "config.agents[0].mappings[0]",
		firstOf(t, agent.(map[string]any)["mappings"]), []string{"agent_path", "scanner_slug"})

	assertKeys(t, "config.logging", document["logging"], []string{
		"endpoint", "server_level", "sink", "stream",
	})

	// _id identifies the stored document rather than describing a setting,
	// so it is the one field deliberately absent from the wire encoding.
	if _, present := document["_id"]; present {
		t.Error("config carries _id on the wire; it belongs only to the stored document")
	}
}

// first returns the first element of the array at document[section][key].
func first(t *testing.T, document map[string]any, section, key string) any {
	t.Helper()
	object, ok := document[section].(map[string]any)
	if !ok {
		t.Fatalf("%s: expected an object, got %T", section, document[section])
	}
	return firstOf(t, object[key])
}

// firstOf returns the first element of value, which must be a non-empty array.
func firstOf(t *testing.T, value any) any {
	t.Helper()
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("expected an array, got %T", value)
	}
	if len(array) == 0 {
		t.Fatal("expected a populated array; the fixture must set every field")
	}
	return array[0]
}

// TestConfigBSONTagsMatchJSONTags guards the other half: the stored document
// and the wire payload have always used the same names, so a change to one
// without the other would make a document written by one build unreadable by
// the next.
//
// The two tags are compared rather than pinned to a list, because their
// agreement is the property that matters and it holds for every field.
func TestConfigBSONTagsMatchJSONTags(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(Config{}),
		reflect.TypeOf(APIKeysConfig{}),
		reflect.TypeOf(APIKeyEntry{}),
		reflect.TypeOf(AdminUser{}),
		reflect.TypeOf(InterfacesConfig{}),
		reflect.TypeOf(SonarrInstance{}),
		reflect.TypeOf(RootDirMapping{}),
		reflect.TypeOf(StorageConfig{}),
		reflect.TypeOf(DirectoryScannerConfig{}),
		reflect.TypeOf(ScanDirectory{}),
		reflect.TypeOf(SidecarTypeDefinition{}),
		reflect.TypeOf(AgentConfig{}),
		reflect.TypeOf(AgentDirectoryMapping{}),
		reflect.TypeOf(LoggingConfig{}),
	}

	for _, structType := range types {
		for i := range structType.NumField() {
			field := structType.Field(i)
			bsonName := tagName(field.Tag.Get("bson"))
			jsonName := tagName(field.Tag.Get("json"))

			// Config.ID is the stored document's _id and is deliberately
			// absent from the wire — the one field where the two disagree.
			if structType == reflect.TypeOf(Config{}) && field.Name == "ID" {
				continue
			}
			if bsonName != jsonName {
				t.Errorf("%s.%s: bson name %q and json name %q disagree; the stored and wire shapes must match",
					structType.Name(), field.Name, bsonName, jsonName)
			}
		}
	}
}

// tagName is the name portion of a struct tag, dropping options like
// omitempty.
func tagName(tag string) string {
	name, _, _ := strings.Cut(tag, ",")
	return name
}

func assertKeys(t *testing.T, where string, value any, want []string) {
	t.Helper()

	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s: expected an object, got %T", where, value)
	}

	got := make([]string, 0, len(object))
	for key := range object {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s keys = %v, want %v", where, got, want)
	}
}
