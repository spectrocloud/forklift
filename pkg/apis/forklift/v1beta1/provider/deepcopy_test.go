package provider

import (
	"testing"

	core "k8s.io/api/core/v1"
)

func TestGeneratedDeepCopy_ProviderPair(t *testing.T) {
	in := &Pair{
		Source:      core.ObjectReference{Namespace: "ns", Name: "src"},
		Destination: core.ObjectReference{Namespace: "ns", Name: "dst"},
	}
	out := in.DeepCopy()
	if out == nil || out == in {
		t.Fatalf("expected non-nil distinct copy: %#v", out)
	}
	if out.Source.Name != "src" || out.Destination.Name != "dst" {
		t.Fatalf("unexpected values: %#v", out)
	}
}
