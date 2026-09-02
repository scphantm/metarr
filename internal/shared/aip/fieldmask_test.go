package aip

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

func TestApplyUpdateMask_EmptyMask(t *testing.T) {
	dst := &metarrv1.SonarrInstance{InstanceName: "keep"}

	for _, mask := range []*fieldmaskpb.FieldMask{nil, {}, {Paths: []string{}}} {
		if err := ApplyUpdateMask(dst, &metarrv1.SonarrInstance{}, mask); !errors.Is(err, ErrEmptyMask) {
			t.Fatalf("mask %v: got %v, want ErrEmptyMask", mask, err)
		}
	}
	if dst.GetInstanceName() != "keep" {
		t.Fatalf("dst mutated on rejected mask: %q", dst.GetInstanceName())
	}
}

func TestApplyUpdateMask_UnknownPath(t *testing.T) {
	cases := []string{"bogus", "storage.bogus", "instance_name.nested"}
	for _, path := range cases {
		err := ApplyUpdateMask(&metarrv1.SonarrInstance{}, &metarrv1.SonarrInstance{}, &fieldmaskpb.FieldMask{Paths: []string{path}})
		if !errors.Is(err, ErrUnknownPath) {
			t.Fatalf("path %q: got %v, want ErrUnknownPath", path, err)
		}
	}
}

func TestApplyUpdateMask_BadPathLeavesDstUntouched(t *testing.T) {
	dst := &metarrv1.SonarrInstance{InstanceName: "keep"}
	src := &metarrv1.SonarrInstance{InstanceName: "new", Storage: &metarrv1.StorageConfig{Ttl: "24h"}}

	// A valid path followed by an invalid one: nothing should apply, and no
	// empty intermediate message should be left on dst.
	err := ApplyUpdateMask(dst, src, &fieldmaskpb.FieldMask{Paths: []string{"instance_name", "storage.bogus"}})
	if !errors.Is(err, ErrUnknownPath) {
		t.Fatalf("got %v, want ErrUnknownPath", err)
	}
	if dst.GetInstanceName() != "keep" {
		t.Fatalf("valid path applied before the mask was rejected: %q", dst.GetInstanceName())
	}
	if dst.Storage != nil {
		t.Fatalf("rejected mask left an intermediate message on dst: %+v", dst.GetStorage())
	}
}

func TestApplyUpdateMask_ScalarCopiesOnlyNamedField(t *testing.T) {
	dst := &metarrv1.SonarrInstance{InstanceName: "original", SonarrUrl: "http://old"}
	src := &metarrv1.SonarrInstance{InstanceName: "ignored", SonarrUrl: "http://new"}

	if err := ApplyUpdateMask(dst, src, &fieldmaskpb.FieldMask{Paths: []string{"sonarr_url"}}); err != nil {
		t.Fatal(err)
	}
	if dst.GetSonarrUrl() != "http://new" {
		t.Fatalf("sonarr_url not updated: %q", dst.GetSonarrUrl())
	}
	if dst.GetInstanceName() != "original" {
		t.Fatalf("instance_name should be untouched, got %q", dst.GetInstanceName())
	}
}

func TestApplyUpdateMask_DottedPathUpdatesOneNestedField(t *testing.T) {
	dst := &metarrv1.SonarrInstance{Storage: &metarrv1.StorageConfig{Mode: "cache", Ttl: "1h", MaxCount: 10}}
	src := &metarrv1.SonarrInstance{Storage: &metarrv1.StorageConfig{Mode: "ignored", Ttl: "24h", MaxCount: 999}}

	if err := ApplyUpdateMask(dst, src, &fieldmaskpb.FieldMask{Paths: []string{"storage.ttl"}}); err != nil {
		t.Fatal(err)
	}
	if dst.GetStorage().GetTtl() != "24h" {
		t.Fatalf("storage.ttl not updated: %q", dst.GetStorage().GetTtl())
	}
	if dst.GetStorage().GetMode() != "cache" || dst.GetStorage().GetMaxCount() != 10 {
		t.Fatalf("storage siblings mutated: %+v", dst.GetStorage())
	}
}

func TestApplyUpdateMask_MessageFieldReplacedWholesale(t *testing.T) {
	dst := &metarrv1.SonarrInstance{Storage: &metarrv1.StorageConfig{Mode: "cache", Ttl: "1h", MaxCount: 10}}
	src := &metarrv1.SonarrInstance{Storage: &metarrv1.StorageConfig{Mode: "off"}}

	if err := ApplyUpdateMask(dst, src, &fieldmaskpb.FieldMask{Paths: []string{"storage"}}); err != nil {
		t.Fatal(err)
	}
	if dst.GetStorage().GetMode() != "off" || dst.GetStorage().GetTtl() != "" || dst.GetStorage().GetMaxCount() != 0 {
		t.Fatalf("storage not replaced wholesale: %+v", dst.GetStorage())
	}

	// The replacement is a deep copy: mutating src afterwards must not reach dst.
	src.Storage.Mode = "mutated"
	if dst.GetStorage().GetMode() != "off" {
		t.Fatalf("dst aliases src sub-message: %q", dst.GetStorage().GetMode())
	}
}

func TestApplyUpdateMask_RepeatedFieldReplacedWholesale(t *testing.T) {
	dst := &metarrv1.SonarrInstance{RootDirMap: []*metarrv1.RootDirMapping{{SonarrPath: "/a", LocalPath: "/x"}}}
	src := &metarrv1.SonarrInstance{RootDirMap: []*metarrv1.RootDirMapping{
		{SonarrPath: "/b", LocalPath: "/y"},
		{SonarrPath: "/c", LocalPath: "/z"},
	}}

	if err := ApplyUpdateMask(dst, src, &fieldmaskpb.FieldMask{Paths: []string{"root_dir_map"}}); err != nil {
		t.Fatal(err)
	}
	if len(dst.GetRootDirMap()) != 2 || dst.GetRootDirMap()[0].GetSonarrPath() != "/b" {
		t.Fatalf("root_dir_map not replaced: %+v", dst.GetRootDirMap())
	}
	src.RootDirMap[0].SonarrPath = "mutated"
	if dst.GetRootDirMap()[0].GetSonarrPath() != "/b" {
		t.Fatalf("dst aliases src list element: %q", dst.GetRootDirMap()[0].GetSonarrPath())
	}
}

func TestApplyUpdateMask_NamedButUnsetFieldClearsDst(t *testing.T) {
	dst := &metarrv1.SonarrInstance{SonarrApiKey: "secret", InstanceName: "keep"}
	src := &metarrv1.SonarrInstance{}

	if err := ApplyUpdateMask(dst, src, &fieldmaskpb.FieldMask{Paths: []string{"sonarr_api_key"}}); err != nil {
		t.Fatal(err)
	}
	if dst.GetSonarrApiKey() != "" {
		t.Fatalf("sonarr_api_key not cleared: %q", dst.GetSonarrApiKey())
	}
	if dst.GetInstanceName() != "keep" {
		t.Fatalf("instance_name mutated: %q", dst.GetInstanceName())
	}
}

func TestApplyUpdateMask_DoesNotMutateCallerMask(t *testing.T) {
	mask := &fieldmaskpb.FieldMask{Paths: []string{"storage.ttl", "storage.mode", "instance_name"}}
	before := proto.Clone(mask).(*fieldmaskpb.FieldMask)

	if err := ApplyUpdateMask(&metarrv1.SonarrInstance{}, &metarrv1.SonarrInstance{}, mask); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(mask, before) {
		t.Fatalf("caller mask mutated: %v -> %v", before.GetPaths(), mask.GetPaths())
	}
}
