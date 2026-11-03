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
// Spectro:  false for vSphere whenever a VDDK image is configured - CDI+VDDK
// copies, virt-v2v only converts in place. Without a VDDK image upstream's
// rules apply. OVA keeps upstream behaviour.
func TestSpectroShouldUseV2vForTransfer(t *testing.T) {
	t.Setenv("VDDK_IMAGE", "")
	newPlan := func(sourceType ProviderType, settings map[string]string) *Plan {
		src, dst := sourceType, OpenShift
		p := &Plan{}
		// Shared disks are migrated, so upstream's rules pick virt-v2v for a
		// cold migration to the host cluster.
		p.Spec.MigrateSharedDisks = true
		p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &src, Settings: settings}}
		p.Referenced.Provider.Destination = &Provider{Spec: ProviderSpec{Type: &dst}}
		return p
	}

	t.Run("vsphere with a provider VDDK image uses CDI", func(t *testing.T) {
		got, err := newPlan(VSphere, map[string]string{VDDK: "vddk:latest"}).ShouldUseV2vForTransfer(ref.Ref{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatal("ShouldUseV2vForTransfer() = true for vSphere with VDDK, want false")
		}
	})

	t.Run("vsphere with a global VDDK image uses CDI", func(t *testing.T) {
		t.Setenv("VDDK_IMAGE", "vddk:latest")
		got, err := newPlan(VSphere, nil).ShouldUseV2vForTransfer(ref.Ref{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatal("ShouldUseV2vForTransfer() = true for vSphere with VDDK_IMAGE, want false")
		}
	})

	t.Run("vsphere without VDDK uses virt-v2v for transfer", func(t *testing.T) {
		got, err := newPlan(VSphere, nil).ShouldUseV2vForTransfer(ref.Ref{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Fatal("ShouldUseV2vForTransfer() = false for vSphere without VDDK, want true")
		}
	})

	t.Run("ova keeps upstream behaviour", func(t *testing.T) {
		got, err := newPlan(Ova, nil).ShouldUseV2vForTransfer(ref.Ref{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Fatal("ShouldUseV2vForTransfer() = false for OVA, want true")
		}
	})
}
