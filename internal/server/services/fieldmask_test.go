package services

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// applyUpdateMask is the one FieldMask application path for the AIP config
// Update methods. These pin the stricter error model ADR-0010 asks for on top
// of go.einride.tech/aip/fieldmask — an empty mask and an unknown path both
// rejected — plus the two shapes a real Update relies on: a dotted path
// touching one nested scalar, and a bare message path replacing the whole
// sub-message.
func TestApplyUpdateMask_RejectsEmptyMask(t *testing.T) {
	for name, mask := range map[string]*fieldmaskpb.FieldMask{
		"nil":       nil,
		"no paths":  {},
		"empty set": {Paths: []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			err := applyUpdateMask(
				&metarrv1.AgentServiceUpsertRequest{},
				&metarrv1.AgentServiceUpsertRequest{},
				mask,
			)
			if !errors.Is(err, errEmptyMask) {
				t.Fatalf("got %v, want errEmptyMask", err)
			}
		})
	}
}

func TestApplyUpdateMask_RejectsUnknownPath(t *testing.T) {
	for name, path := range map[string]string{
		"no such field":          "agent.nickname",
		"descend through scalar": "agent.display_name.value",
		"top-level typo":         "agents",
	} {
		t.Run(name, func(t *testing.T) {
			err := applyUpdateMask(
				&metarrv1.AgentServiceUpsertRequest{},
				&metarrv1.AgentServiceUpsertRequest{},
				&fieldmaskpb.FieldMask{Paths: []string{path}},
			)
			if !errors.Is(err, errUnknownPath) {
				t.Fatalf("got %v, want errUnknownPath", err)
			}
		})
	}
}

func TestApplyUpdateMask_DottedPathTouchesOnlyTheNamedScalar(t *testing.T) {
	dst := &metarrv1.AgentServiceUpsertRequest{Agent: &metarrv1.AgentConfig{
		Slug:        "nas-01",
		DisplayName: "old",
		LogLevel:    "info",
	}}
	src := &metarrv1.AgentServiceUpsertRequest{Agent: &metarrv1.AgentConfig{
		Slug:        "ignored",
		DisplayName: "new",
		LogLevel:    "debug",
	}}

	if err := applyUpdateMask(dst, src, &fieldmaskpb.FieldMask{Paths: []string{"agent.display_name"}}); err != nil {
		t.Fatalf("applyUpdateMask: %v", err)
	}
	if dst.GetAgent().GetDisplayName() != "new" {
		t.Errorf("display_name = %q, want the merged %q", dst.GetAgent().GetDisplayName(), "new")
	}
	if dst.GetAgent().GetSlug() != "nas-01" || dst.GetAgent().GetLogLevel() != "info" {
		t.Errorf("a dotted-path update moved an unnamed sibling: %+v", dst.GetAgent())
	}
}

func TestApplyUpdateMask_BareMessagePathReplacesTheWholeSubMessage(t *testing.T) {
	dst := &metarrv1.AgentServiceUpsertRequest{Agent: &metarrv1.AgentConfig{
		Slug:        "nas-01",
		DisplayName: "old",
		LogLevel:    "info",
		Mappings:    []*metarrv1.AgentDirectoryMapping{{ScannerSlug: "movies", AgentPath: "/old"}},
	}}
	src := &metarrv1.AgentServiceUpsertRequest{Agent: &metarrv1.AgentConfig{
		Slug:        "nas-02",
		DisplayName: "new",
	}}

	if err := applyUpdateMask(dst, src, &fieldmaskpb.FieldMask{Paths: []string{"agent"}}); err != nil {
		t.Fatalf("applyUpdateMask: %v", err)
	}
	if dst.GetAgent().GetSlug() != "nas-02" || dst.GetAgent().GetDisplayName() != "new" {
		t.Errorf("agent was not replaced by src: %+v", dst.GetAgent())
	}
	if dst.GetAgent().GetLogLevel() != "" || len(dst.GetAgent().GetMappings()) != 0 {
		t.Errorf("a bare message-path update kept fields absent from src: %+v", dst.GetAgent())
	}
}
