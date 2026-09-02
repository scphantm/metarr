package aip

import (
	"errors"
	"testing"

	"Metarr/internal/shared/appconfig"
)

func TestSingleSegmentNamesRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		format func(string) string
		parse  func(string) (string, error)
		want   string
	}{
		{"agents", AgentName, ParseAgentName, "agents/nas-01"},
		{"sonarrInstances", SonarrInstanceName, ParseSonarrInstanceName, "sonarrInstances/main"},
		{"scanDirectories", ScanDirectoryName, ParseScanDirectoryName, "scanDirectories/movies"},
		{"sidecarTypes", SidecarTypeName, ParseSidecarTypeName, "sidecarTypes/01HZ"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := tc.want[len(tc.name)+1:]
			if got := tc.format(id); got != tc.want {
				t.Fatalf("format(%q) = %q, want %q", id, got, tc.want)
			}
			gotID, err := tc.parse(tc.want)
			if err != nil {
				t.Fatalf("parse(%q): %v", tc.want, err)
			}
			if gotID != id {
				t.Fatalf("parse(%q) = %q, want %q", tc.want, gotID, id)
			}
		})
	}
}

func TestSingleSegmentNamesReject(t *testing.T) {
	bad := []string{"", "nas-01", "agents/", "agents/a/b", "sonarrInstances/main", "sidecar_types/x"}
	for _, name := range bad {
		if _, err := ParseAgentName(name); !errors.Is(err, ErrMalformedName) {
			t.Fatalf("ParseAgentName(%q) = %v, want ErrMalformedName", name, err)
		}
	}
}

func TestAPIKeyNameRoundTripsEveryAccessLevel(t *testing.T) {
	levels := []appconfig.APIKeyGroup{
		appconfig.APIKeyGroupAdmin,
		appconfig.APIKeyGroupUser,
		appconfig.APIKeyGroupWebhook,
		appconfig.APIKeyGroupReadOnly,
	}
	for _, level := range levels {
		name := APIKeyName(level, "key-1")
		gotLevel, gotID, err := ParseAPIKeyName(name)
		if err != nil {
			t.Fatalf("ParseAPIKeyName(%q): %v", name, err)
		}
		if gotLevel != level || gotID != "key-1" {
			t.Fatalf("ParseAPIKeyName(%q) = (%q, %q), want (%q, key-1)", name, gotLevel, gotID, level)
		}

		parent := AccessLevelName(level)
		gotParentLevel, err := ParseAPIKeyParent(parent)
		if err != nil {
			t.Fatalf("ParseAPIKeyParent(%q): %v", parent, err)
		}
		if gotParentLevel != level {
			t.Fatalf("ParseAPIKeyParent(%q) = %q, want %q", parent, gotParentLevel, level)
		}
	}
}

func TestAPIKeyNameRejectsBadAccessLevel(t *testing.T) {
	if _, _, err := ParseAPIKeyName("accessLevels/superuser/apiKeys/k1"); !errors.Is(err, ErrMalformedName) {
		t.Fatalf("bad level in name: got %v, want ErrMalformedName", err)
	}
	if _, err := ParseAPIKeyParent("accessLevels/superuser"); !errors.Is(err, ErrMalformedName) {
		t.Fatalf("bad level in parent: got %v, want ErrMalformedName", err)
	}
}

func TestAPIKeyNameRejectsMalformedShape(t *testing.T) {
	bad := []string{
		"accessLevels/admin/apiKeys",
		"accessLevels/admin/apiKeys/",
		"accessLevels//apiKeys/k1",
		"accessLevels/admin/keys/k1",
		"apiKeys/k1",
		"accessLevels/admin/apiKeys/k1/extra",
	}
	for _, name := range bad {
		if _, _, err := ParseAPIKeyName(name); !errors.Is(err, ErrMalformedName) {
			t.Fatalf("ParseAPIKeyName(%q) = %v, want ErrMalformedName", name, err)
		}
	}
}
