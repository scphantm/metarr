package appconfig

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
)

// The config document is stored and transported through MarshalStored /
// UnmarshalStored — protojson with proto field names and every field
// emitted. These tests pin the two properties that gives the stored
// document: the field names an operator inspecting the collection sees are
// the snake_case proto names, and a document written and read back is
// unchanged.

// TestMarshalStoredUsesProtoFieldNames pins the serialized key names for the
// config document. protojson defaults to camelCase; UseProtoNames is what
// keeps api_keys as api_keys, and a regression there changes what is written
// to the database.
func TestMarshalStoredUsesProtoFieldNames(t *testing.T) {
	config := Default()
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

	encoded, err := MarshalStored(config)
	if err != nil {
		t.Fatalf("marshalling the config: %v", err)
	}

	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decoding the marshalled config: %v", err)
	}

	assertKeys(t, "config", document, []string{
		"admin", "agents", "api_keys", "directory_scanner", "event_bus", "interfaces", "logging",
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
	// name is the AIP resource-name field (ADR-0010). It is derived on read
	// and cleared by the config store's mutation closure before a write, so
	// a stored entry only ever carries it empty — EmitUnpopulated still
	// lists the key, which is why it appears here.
	assertKeys(t, "config.directory_scanner.sidecar_types[0]",
		first(t, document, "directory_scanner", "sidecar_types"),
		[]string{"category", "extensions", "id", "name", "order", "patterns", "type"})

	agent := firstOf(t, document["agents"])
	assertKeys(t, "config.agents[0]", agent, []string{
		"display_name", "log_level", "mappings", "slug",
	})
	assertKeys(t, "config.agents[0].mappings[0]",
		firstOf(t, agent.(map[string]any)["mappings"]), []string{"agent_path", "scanner_slug"})

	assertKeys(t, "config.logging", document["logging"], []string{
		"endpoint", "server_level", "sink", "stream",
	})

	assertKeys(t, "config.event_bus", document["event_bus"], []string{
		"max_len", "retention_hours",
		"retry_attempts", "retry_backoff_base_ms", "retry_backoff_max_ms",
	})

	// _id identifies the stored document rather than describing a setting,
	// so it is deliberately not a field on the message and never in the
	// encoding.
	if _, present := document["_id"]; present {
		t.Error("config carries _id in its encoding; that belongs only to the stored document envelope")
	}
}

// TestMarshalStoredEmitsUnpopulatedFields is the "lists every setting"
// property: a freshly defaulted config with an empty logging section still
// serializes every logging key, so the stored document is self-describing
// rather than showing only the fields that differ from zero.
func TestMarshalStoredEmitsUnpopulatedFields(t *testing.T) {
	config := Default()
	config.Logging = &LoggingConfig{} // every field at its zero value

	encoded, err := MarshalStored(config)
	if err != nil {
		t.Fatalf("marshalling the config: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	assertKeys(t, "config.logging", document["logging"], []string{
		"endpoint", "server_level", "sink", "stream",
	})
}

// TestStoredRoundTrip is the store-level property the Mongo repo and the
// config-update event payload both rely on: a config marshalled and
// unmarshalled again is the same config.
func TestStoredRoundTrip(t *testing.T) {
	original := Default()
	original.Admin = &AdminUser{Username: "admin", Email: "a@b.c", PasswordSalt: "s", PasswordHash: "h"}
	original.ApiKeys.User = []*APIKeyEntry{{Id: "u1", Name: "ci", ApiKey: "secret"}}
	original.Interfaces.Sonarr = []*SonarrInstance{{
		InstanceSlug: "main", SonarrUrl: "http://sonarr", SonarrApiKey: "k",
		Storage: &StorageConfig{Mode: "versioned", MaxCount: 5},
	}}
	original.Logging = &LoggingConfig{ServerLevel: LogLevelDebug, Sink: "openobserve"}

	encoded, err := MarshalStored(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded, err := UnmarshalStored(encoded)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatalf("round trip changed the document:\n got %v\nwant %v", decoded, original)
	}
}

func first(t *testing.T, document map[string]any, section, key string) any {
	t.Helper()
	object, ok := document[section].(map[string]any)
	if !ok {
		t.Fatalf("%s: expected an object, got %T", section, document[section])
	}
	return firstOf(t, object[key])
}

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
