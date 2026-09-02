package aip

import (
	"testing"

	busv1 "Metarr/internal/genproto/metarr/bus/v1"
	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

func TestClearDerivedStripsResourceName(t *testing.T) {
	sidecar := &busv1.SidecarTypeDefinition{Id: "01HZ", Type: "poster", Name: "sidecarTypes/01HZ"}
	agent := &metarrv1.AgentConfig{Slug: "nas-01"}

	ClearDerived(sidecar, agent, nil)

	if sidecar.GetName() != "" {
		t.Fatalf("name not cleared: %q", sidecar.GetName())
	}
	if sidecar.GetId() != "01HZ" || sidecar.GetType() != "poster" {
		t.Fatalf("non-derived fields mutated: %+v", sidecar)
	}
}

func TestClearDerivedSliceStripsEveryElement(t *testing.T) {
	sidecars := []*busv1.SidecarTypeDefinition{
		{Id: "a", Name: "sidecarTypes/a"},
		{Id: "b", Name: "sidecarTypes/b"},
	}
	ClearDerivedSlice(sidecars)
	for _, s := range sidecars {
		if s.GetName() != "" {
			t.Fatalf("name not cleared on %q", s.GetId())
		}
	}
}
