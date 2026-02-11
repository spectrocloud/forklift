package v1beta1

import (
	"testing"

	plan "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	providerapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/provider"
	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGeneratedDeepCopy_V1beta1_MoreCoverage(t *testing.T) {
	// Exercise remaining DeepCopy() paths that were still 0% in coverprofile.
	ms := (&MigrationSpec{
		Plan:   core.ObjectReference{Namespace: "ns", Name: "p"},
		Cancel: []refapi.Ref{{ID: "vm-1"}},
	}).DeepCopy()
	if ms == nil || ms.Plan.Name != "p" || len(ms.Cancel) != 1 || ms.Cancel[0].ID != "vm-1" {
		t.Fatalf("unexpected MigrationSpec deepcopy: %#v", ms)
	}

	nms := (&NetworkMapSpec{
		Provider: providerapi.Pair{
			Source:      core.ObjectReference{Namespace: "ns", Name: "src"},
			Destination: core.ObjectReference{Namespace: "ns", Name: "dst"},
		},
		Map: []NetworkPair{
			{Source: refapi.Ref{ID: "net-1"}, Destination: DestinationNetwork{Type: "pod"}},
		},
	}).DeepCopy()
	if nms == nil || nms.Provider.Source.Name != "src" || len(nms.Map) != 1 || nms.Map[0].Source.ID != "net-1" {
		t.Fatalf("unexpected NetworkMapSpec deepcopy: %#v", nms)
	}

	off := (&OffloadPlugin{VSphereXcopyPluginConfig: &VSphereXcopyPluginConfig{SecretRef: "s", StorageVendorProduct: StorageVendorProductOntap}}).DeepCopy()
	if off == nil || off.VSphereXcopyPluginConfig == nil || off.VSphereXcopyPluginConfig.SecretRef != "s" {
		t.Fatalf("unexpected OffloadPlugin deepcopy: %#v", off)
	}

	sms := (&StorageMapSpec{
		Provider: providerapi.Pair{
			Source:      core.ObjectReference{Namespace: "ns", Name: "src"},
			Destination: core.ObjectReference{Namespace: "ns", Name: "dst"},
		},
		Map: []StoragePair{
			{Source: refapi.Ref{ID: "ds-1"}, Destination: DestinationStorage{StorageClass: "sc"}, OffloadPlugin: off},
		},
	}).DeepCopy()
	if sms == nil || sms.Provider.Source.Name != "src" || len(sms.Map) != 1 || sms.Map[0].Destination.StorageClass != "sc" {
		t.Fatalf("unexpected StorageMapSpec deepcopy: %#v", sms)
	}

	cfg := (&VSphereXcopyPluginConfig{SecretRef: "s", StorageVendorProduct: StorageVendorProductOntap}).DeepCopy()
	if cfg == nil || cfg.SecretRef != "s" || cfg.StorageVendorProduct != StorageVendorProductOntap {
		t.Fatalf("unexpected VSphereXcopyPluginConfig deepcopy: %#v", cfg)
	}

	// Exercise Referenced helpers.
	var r Referenced
	h := &Hook{}
	h.Namespace = "ns"
	h.Name = "h1"
	r.Hooks = []*Hook{h}
	found, got := r.FindHook(core.ObjectReference{Namespace: "ns", Name: "h1"})
	if !found || got == nil || got.Name != "h1" {
		t.Fatalf("unexpected FindHook: found=%v hook=%#v", found, got)
	}

	// DeepCopyInto is a no-op by design, but call it to cover the stub.
	var out Referenced
	r.DeepCopyInto(&out)
	if r.DeepCopy() != &r {
		t.Fatalf("expected DeepCopy to return receiver")
	}

	// Touch plan package type use to keep imports stable.
	_ = (&plan.VMStatus{}).DeepCopy()

	// Remaining generated deepcopies that were still 0% in coverprofile.
	if (&MigrationStatus{}).DeepCopy() == nil {
		t.Fatalf("expected MigrationStatus.DeepCopy to return non-nil")
	}
	if (&VSphereXcopyVolumePopulatorSpec{}).DeepCopy() == nil {
		t.Fatalf("expected VSphereXcopyVolumePopulatorSpec.DeepCopy to return non-nil")
	}
	if (&VSphereXcopyVolumePopulatorStatus{}).DeepCopy() == nil {
		t.Fatalf("expected VSphereXcopyVolumePopulatorStatus.DeepCopy to return non-nil")
	}
}

// ---- Consolidated from plan_more_test.go ----

func TestPlanSpec_FindVM_FoundByID(t *testing.T) {
	s := &PlanSpec{
		VMs: []plan.VM{
			{Ref: refapi.Ref{ID: "a"}},
			{Ref: refapi.Ref{ID: "b"}},
		},
	}
	vm, found := s.FindVM(refapi.Ref{ID: "b"})
	if !found || vm == nil || vm.ID != "b" {
		t.Fatalf("expected found vm b, got found=%v vm=%#v", found, vm)
	}
}

func TestPlanSpec_FindVM_NotFound(t *testing.T) {
	s := &PlanSpec{VMs: []plan.VM{{Ref: refapi.Ref{ID: "a"}}}}
	vm, found := s.FindVM(refapi.Ref{ID: "x"})
	if found || vm != nil {
		t.Fatalf("expected not found")
	}
}

func TestPlan_ShouldUseV2vForTransfer_ErrWhenSourceMissing(t *testing.T) {
	p := &Plan{}
	_, err := p.ShouldUseV2vForTransfer()
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestPlan_ShouldUseV2vForTransfer_ErrWhenDestinationMissing(t *testing.T) {
	p := &Plan{}
	srcType := VSphere
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &srcType}}
	_, err := p.ShouldUseV2vForTransfer()
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestPlan_ShouldUseV2vForTransfer_OvaAlwaysTrue(t *testing.T) {
	p := &Plan{}
	srcType := Ova
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &srcType}}
	dstType := OpenShift
	p.Referenced.Provider.Destination = &Provider{Spec: ProviderSpec{Type: &dstType, URL: ""}}
	ok, err := p.ShouldUseV2vForTransfer()
	if err != nil || !ok {
		t.Fatalf("expected true nil, got %v %v", ok, err)
	}
}

func TestPlan_ShouldUseV2vForTransfer_VSphere_TrueWhenColdHostSharedAndNotSkip(t *testing.T) {
	p := &Plan{Spec: PlanSpec{Warm: false, MigrateSharedDisks: true, SkipGuestConversion: false}}
	srcType := VSphere
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &srcType}}
	dstType := OpenShift
	p.Referenced.Provider.Destination = &Provider{Spec: ProviderSpec{Type: &dstType, URL: ""}}
	ok, err := p.ShouldUseV2vForTransfer()
	if err != nil || !ok {
		t.Fatalf("expected true nil, got %v %v", ok, err)
	}
}

func TestPlan_ShouldUseV2vForTransfer_VSphere_FalseWhenWarm(t *testing.T) {
	p := &Plan{Spec: PlanSpec{Warm: true, MigrateSharedDisks: true, SkipGuestConversion: false}}
	srcType := VSphere
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &srcType}}
	dstType := OpenShift
	p.Referenced.Provider.Destination = &Provider{Spec: ProviderSpec{Type: &dstType, URL: ""}}
	ok, err := p.ShouldUseV2vForTransfer()
	if err != nil || ok {
		t.Fatalf("expected false nil, got %v %v", ok, err)
	}
}

func TestPlan_ShouldUseV2vForTransfer_VSphere_FalseWhenDestNotHost(t *testing.T) {
	p := &Plan{Spec: PlanSpec{Warm: false, MigrateSharedDisks: true, SkipGuestConversion: false}}
	srcType := VSphere
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &srcType}}
	dstType := OpenShift
	// URL non-empty => not host.
	p.Referenced.Provider.Destination = &Provider{Spec: ProviderSpec{Type: &dstType, URL: "https://x"}}
	ok, err := p.ShouldUseV2vForTransfer()
	if err != nil || ok {
		t.Fatalf("expected false nil, got %v %v", ok, err)
	}
}

func TestPlan_ShouldUseV2vForTransfer_VSphere_FalseWhenNotMigrateSharedDisks(t *testing.T) {
	p := &Plan{Spec: PlanSpec{Warm: false, MigrateSharedDisks: false, SkipGuestConversion: false}}
	srcType := VSphere
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &srcType}}
	dstType := OpenShift
	p.Referenced.Provider.Destination = &Provider{Spec: ProviderSpec{Type: &dstType, URL: ""}}
	ok, err := p.ShouldUseV2vForTransfer()
	if err != nil || ok {
		t.Fatalf("expected false nil, got %v %v", ok, err)
	}
}

func TestPlan_ShouldUseV2vForTransfer_VSphere_FalseWhenSkipGuestConversion(t *testing.T) {
	p := &Plan{Spec: PlanSpec{Warm: false, MigrateSharedDisks: true, SkipGuestConversion: true}}
	srcType := VSphere
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &srcType}}
	dstType := OpenShift
	p.Referenced.Provider.Destination = &Provider{Spec: ProviderSpec{Type: &dstType, URL: ""}}
	ok, err := p.ShouldUseV2vForTransfer()
	if err != nil || ok {
		t.Fatalf("expected false nil, got %v %v", ok, err)
	}
}

func TestPlan_ShouldUseV2vForTransfer_DefaultFalseForUnknownSource(t *testing.T) {
	p := &Plan{}
	srcType := OpenStack
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &srcType}}
	dstType := OpenShift
	p.Referenced.Provider.Destination = &Provider{Spec: ProviderSpec{Type: &dstType, URL: ""}}
	ok, err := p.ShouldUseV2vForTransfer()
	if err != nil || ok {
		t.Fatalf("expected false nil, got %v %v", ok, err)
	}
}

func TestPlan_IsSourceProviderHelpers_More(t *testing.T) {
	p := &Plan{}
	// VSphere
	tp := VSphere
	p.Referenced.Provider.Source = &Provider{Spec: ProviderSpec{Type: &tp}}
	if !p.IsSourceProviderVSphere() || p.IsSourceProviderOCP() || p.IsSourceProviderOVA() {
		t.Fatalf("unexpected helper results")
	}
	// OVA
	tp = Ova
	p.Referenced.Provider.Source.Spec.Type = &tp
	if !p.IsSourceProviderOVA() {
		t.Fatalf("expected ova true")
	}
	// OpenShift
	tp = OpenShift
	p.Referenced.Provider.Source.Spec.Type = &tp
	if !p.IsSourceProviderOCP() {
		t.Fatalf("expected ocp true")
	}
	// OpenStack
	tp = OpenStack
	p.Referenced.Provider.Source.Spec.Type = &tp
	if !p.IsSourceProviderOpenstack() {
		t.Fatalf("expected openstack true")
	}
	// OVirt
	tp = OVirt
	p.Referenced.Provider.Source.Spec.Type = &tp
	if !p.IsSourceProviderOvirt() {
		t.Fatalf("expected ovirt true")
	}
}

// ---- Consolidated from provider_more_test.go ----

func TestProvider_Type_UndefinedWhenNil(t *testing.T) {
	p := &Provider{}
	if p.Type() != Undefined {
		t.Fatalf("expected Undefined, got %v", p.Type())
	}
}

func TestProvider_Type_ReturnsSetType(t *testing.T) {
	tp := OpenStack
	p := &Provider{Spec: ProviderSpec{Type: &tp}}
	if p.Type() != OpenStack {
		t.Fatalf("expected OpenStack, got %v", p.Type())
	}
}

func TestProvider_IsHost_TrueWhenOpenShiftAndEmptyURL(t *testing.T) {
	tp := OpenShift
	p := &Provider{Spec: ProviderSpec{Type: &tp, URL: ""}}
	if !p.IsHost() {
		t.Fatalf("expected host")
	}
}

func TestProvider_IsHost_FalseWhenOpenShiftAndURLSet(t *testing.T) {
	tp := OpenShift
	p := &Provider{Spec: ProviderSpec{Type: &tp, URL: "https://x"}}
	if p.IsHost() {
		t.Fatalf("expected not host")
	}
}

func TestProvider_IsHost_FalseWhenNotOpenShift(t *testing.T) {
	tp := VSphere
	p := &Provider{Spec: ProviderSpec{Type: &tp, URL: ""}}
	if p.IsHost() {
		t.Fatalf("expected not host")
	}
}

func TestProvider_IsRestrictedHost_TrueWhenDifferentNamespaceFromEnv(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "ns-env")
	tp := OpenShift
	p := &Provider{Spec: ProviderSpec{Type: &tp, URL: ""}}
	p.Namespace = "ns-other"
	if !p.IsRestrictedHost() {
		t.Fatalf("expected restricted host")
	}
}

func TestProvider_IsRestrictedHost_FalseWhenSameNamespaceAsEnv(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "ns-env")
	tp := OpenShift
	p := &Provider{Spec: ProviderSpec{Type: &tp, URL: ""}}
	p.Namespace = "ns-env"
	if p.IsRestrictedHost() {
		t.Fatalf("expected not restricted")
	}
}

func TestProvider_IsRestrictedHost_FalseWhenNotHost(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "ns-env")
	tp := OpenShift
	p := &Provider{Spec: ProviderSpec{Type: &tp, URL: "https://x"}}
	p.Namespace = "ns-other"
	if p.IsRestrictedHost() {
		t.Fatalf("expected not restricted")
	}
}

func TestProvider_HasReconciled_TrueWhenObservedMatchesGeneration(t *testing.T) {
	p := &Provider{}
	p.Generation = 3
	p.Status.ObservedGeneration = 3
	if !p.HasReconciled() {
		t.Fatalf("expected reconciled")
	}
}

func TestProvider_HasReconciled_FalseWhenObservedDoesNotMatchGeneration(t *testing.T) {
	p := &Provider{}
	p.Generation = 3
	p.Status.ObservedGeneration = 2
	if p.HasReconciled() {
		t.Fatalf("expected not reconciled")
	}
}

func TestProvider_RequiresConversion_TrueWhenConvertDiskEnabled(t *testing.T) {
	enabled := true
	p := &Provider{Spec: ProviderSpec{ConvertDisk: &enabled}}
	if !p.RequiresConversion() {
		t.Fatalf("expected conversion required")
	}
}

func TestProvider_RequiresConversion_FalseWhenConvertDiskNilOrFalse(t *testing.T) {
	p := &Provider{Spec: ProviderSpec{ConvertDisk: nil}}
	if p.RequiresConversion() {
		t.Fatalf("expected no conversion when ConvertDisk is nil")
	}
	disabled := false
	p2 := &Provider{Spec: ProviderSpec{ConvertDisk: &disabled}}
	if p2.RequiresConversion() {
		t.Fatalf("expected no conversion when ConvertDisk is false")
	}
}

func TestProvider_UseVddkAioOptimization_DefaultFalse(t *testing.T) {
	p := &Provider{}
	if p.UseVddkAioOptimization() {
		t.Fatalf("expected false")
	}
}

// ---- Consolidated from referenced_deepcopy_more_test.go ----

func TestReferenced_DeepCopy_ReturnsSelf(t *testing.T) {
	in := &Referenced{}
	out := in.DeepCopy()
	if out != in {
		t.Fatalf("expected same pointer")
	}
}

func TestReferenced_DeepCopyInto_NoPanic_More(t *testing.T) {
	in := &Referenced{}
	out := &Referenced{}
	in.DeepCopyInto(out)
}

// ---- Consolidated from referenced_findhook_more_test.go ----

func TestReferenced_FindHook_Found(t *testing.T) {
	in := &Referenced{
		Hooks: []*Hook{
			{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h1"}},
			{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h2"}},
		},
	}
	found, hook := in.FindHook(core.ObjectReference{Namespace: "ns", Name: "h2"})
	if !found || hook == nil || hook.Name != "h2" {
		t.Fatalf("expected found h2, got found=%v hook=%#v", found, hook)
	}
}

func TestReferenced_FindHook_NotFound(t *testing.T) {
	in := &Referenced{
		Hooks: []*Hook{
			{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h1"}},
		},
	}
	found, hook := in.FindHook(core.ObjectReference{Namespace: "ns", Name: "missing"})
	_ = hook
	if found {
		t.Fatalf("expected not found")
	}
}
