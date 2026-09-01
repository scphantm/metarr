package busstats

import "testing"

// realInfoSample is a trimmed but otherwise verbatim INFO response, kept in
// the wire format: CRLF line endings, section headers, and a blank line
// between sections.
const realInfoSample = "# Server\r\n" +
	"redis_version:7.2.4\r\n" +
	"uptime_in_seconds:86400\r\n" +
	"\r\n" +
	"# Clients\r\n" +
	"connected_clients:6\r\n" +
	"\r\n" +
	"# Memory\r\n" +
	"used_memory:1048576\r\n" +
	"used_memory_human:1.00M\r\n" +
	"\r\n" +
	"# Stats\r\n" +
	"instantaneous_ops_per_sec:42\r\n" +
	"\r\n" +
	"# Keyspace\r\n" +
	"db0:keys=17,expires=2,avg_ttl=0\r\n"

func TestParseInfoReadsFieldsAcrossSections(t *testing.T) {
	fields := parseInfo(realInfoSample)

	want := map[string]string{
		"redis_version":             "7.2.4",
		"uptime_in_seconds":         "86400",
		"connected_clients":         "6",
		"used_memory":               "1048576",
		"used_memory_human":         "1.00M",
		"instantaneous_ops_per_sec": "42",
	}

	for key, expected := range want {
		if got := fields[key]; got != expected {
			t.Errorf("fields[%q] = %q, want %q", key, got, expected)
		}
	}
}

func TestParseInfoSkipsSectionHeadersAndBlankLines(t *testing.T) {
	fields := parseInfo(realInfoSample)

	if _, ok := fields["# Server"]; ok {
		t.Error("section header was parsed as a field")
	}
	if _, ok := fields[""]; ok {
		t.Error("blank line was parsed as a field")
	}
}

// Keyspace lines carry colons inside the value. Splitting on every colon
// rather than only the first would truncate them.
func TestParseInfoKeepsColonsInsideValues(t *testing.T) {
	fields := parseInfo(realInfoSample)

	if got, want := fields["db0"], "keys=17,expires=2,avg_ttl=0"; got != want {
		t.Errorf("fields[\"db0\"] = %q, want %q", got, want)
	}
}

func TestParseInfoIgnoresLinesWithoutASeparator(t *testing.T) {
	fields := parseInfo("garbage\r\nredis_version:7.2.4\r\n")

	if len(fields) != 1 {
		t.Fatalf("parsed %d fields, want 1: %v", len(fields), fields)
	}
	if fields["redis_version"] != "7.2.4" {
		t.Errorf("redis_version = %q, want 7.2.4", fields["redis_version"])
	}
}

func TestInfoIntParsesAndDefaultsToZero(t *testing.T) {
	fields := parseInfo(realInfoSample)

	if got := infoInt(fields, "connected_clients"); got != 6 {
		t.Errorf("connected_clients = %d, want 6", got)
	}
	// A counter the running server does not report must read as zero rather
	// than fail the surrounding snapshot.
	if got := infoInt(fields, "not_reported_by_this_version"); got != 0 {
		t.Errorf("missing key = %d, want 0", got)
	}
	// Neither should a non-numeric value.
	if got := infoInt(fields, "used_memory_human"); got != 0 {
		t.Errorf("non-numeric value = %d, want 0", got)
	}
}
