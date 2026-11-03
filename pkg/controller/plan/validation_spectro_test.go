package plan

import (
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/settings"
	"k8s.io/utils/ptr"
)

// Spectro: without a VDDK image virt-v2v copies the disks as part of
// conversion, and conversion is opt-in via Provider.Spec.ConvertDisk, so a
// vSphere plan without VDDK must require convertDisk.
func TestSpectroValidateVddkImageWithoutVddkRequiresConvertDisk(t *testing.T) {
	saved := settings.Settings.Migration.VddkImage
	settings.Settings.Migration.VddkImage = ""
	t.Cleanup(func() { settings.Settings.Migration.VddkImage = saved })

	newPlan := func(convertDisk *bool) *api.Plan {
		src, dst := api.VSphere, api.OpenShift
		p := &api.Plan{}
		p.Referenced.Provider.Source = &api.Provider{Spec: api.ProviderSpec{Type: &src, ConvertDisk: convertDisk}}
		p.Referenced.Provider.Destination = &api.Provider{Spec: api.ProviderSpec{Type: &dst}}
		return p
	}

	for _, tc := range []struct {
		name        string
		convertDisk *bool
		wantBlocked bool
	}{
		{"convertDisk unset", nil, true},
		{"convertDisk false", ptr.To(false), true},
		{"convertDisk true", ptr.To(true), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPlan(tc.convertDisk)
			if err := (&Reconciler{}).validateVddkImage(p); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := p.Status.HasCondition(VDDKInitImageUnavailable); got != tc.wantBlocked {
				t.Fatalf("%s condition = %v, want %v", VDDKInitImageUnavailable, got, tc.wantBlocked)
			}
		})
	}
}
