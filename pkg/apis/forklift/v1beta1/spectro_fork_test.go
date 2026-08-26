package v1beta1

import (
	"testing"

	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	"k8s.io/utils/ptr"
)

// These tests guard the two behavioural inversions the Spectro fork carries on
// top of upstream. Upstream has no coverage for either function, so without
// these an upstream refactor can silently restore upstream semantics.
//
// See .claude/skills/forklift-spectro-delta/ for the full delta inventory.

// Upstream: RequiresConversion() is true for VSphere/Ova/HyperV/EC2.
// Spectro:  conversion is opt-in via Provider.Spec.ConvertDisk.
func TestSpectroRequiresConversionIsOptIn(t *testing.T) {
	for _, tc := range []struct {
		name        string
		convertDisk *bool
		want        bool
	}{
		{"unset defaults to no conversion", nil, false},
		{"explicit false", ptr.To(false), false},
		{"explicit true", ptr.To(true), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vsphere := VSphere
			p := &Provider{Spec: ProviderSpec{Type: &vsphere, ConvertDisk: tc.convertDisk}}
			if got := p.RequiresConversion(); got != tc.want {
				t.Fatalf("RequiresConversion() = %v, want %v (ConvertDisk=%v)", got, tc.want, tc.convertDisk)
			}
		})
	}
}

// Upstream: ShouldUseV2vForTransfer() can return true for vSphere, letting
// virt-v2v copy the disks.
// Spectro:  always false for vSphere - CDI+VDDK copies, virt-v2v only converts
// in place. OVA keeps upstream behaviour.
func TestSpectroShouldUseV2vForTransfer(t *testing.T) {
	newPlan := func(sourceType ProviderType) *Plan {
		src, dst := sourceType, OpenShift
		p := &Plan{}
		p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &src}}
		p.Referenced.Provider.Destination = &Provider{Spec: ProviderSpec{Type: &dst}}
		return p
	}

	t.Run("vsphere never uses virt-v2v for transfer", func(t *testing.T) {
		got, err := newPlan(VSphere).ShouldUseV2vForTransfer(ref.Ref{}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatal("ShouldUseV2vForTransfer() = true for vSphere, want false")
		}
	})

	t.Run("ova keeps upstream behaviour", func(t *testing.T) {
		got, err := newPlan(Ova).ShouldUseV2vForTransfer(ref.Ref{}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Fatal("ShouldUseV2vForTransfer() = false for OVA, want true")
		}
	})
}
