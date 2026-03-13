package plan

import (
	"testing"
	"time"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	"github.com/kubev2v/forklift/pkg/settings"
	batchv1 "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateTargetNamespace_NotSet_SetsCondition(t *testing.T) {
	r := createFakeReconciler()
	p := &api.Plan{}
	p.Spec.TargetNamespace = ""
	_ = r.validateTargetNamespace(p)
	if !p.Status.HasCondition(NamespaceNotValid) {
		t.Fatalf("expected condition")
	}
	c := p.Status.FindCondition(NamespaceNotValid)
	if c == nil || c.Reason != NotSet {
		t.Fatalf("expected NotSet reason, got %#v", c)
	}
}

func TestValidateTargetNamespace_InvalidDNS1123_SetsCondition(t *testing.T) {
	r := createFakeReconciler()
	p := &api.Plan{}
	p.Spec.TargetNamespace = "bad_name"
	_ = r.validateTargetNamespace(p)
	c := p.Status.FindCondition(NamespaceNotValid)
	if c == nil || c.Reason != NotValid {
		t.Fatalf("expected NotValid reason, got %#v", c)
	}
}

func TestValidateTargetNamespace_Valid_NoCondition(t *testing.T) {
	r := createFakeReconciler()
	p := &api.Plan{}
	p.Spec.TargetNamespace = "good-ns"
	_ = r.validateTargetNamespace(p)
	if p.Status.HasCondition(NamespaceNotValid) {
		t.Fatalf("expected no condition")
	}
}

func TestValidateVolumeNameTemplate_Invalid_SetsNotValidCondition(t *testing.T) {
	r := createFakeReconciler()
	p := &api.Plan{}
	p.Spec.VolumeNameTemplate = "Bad"
	_ = r.validateVolumeNameTemplate(p)
	if !p.Status.HasCondition(NotValid) {
		t.Fatalf("expected NotValid condition")
	}
}

func TestValidateVolumeNameTemplate_Valid_NoCondition(t *testing.T) {
	r := createFakeReconciler()
	p := &api.Plan{}
	p.Spec.VolumeNameTemplate = "{{ .PVCName }}-{{ .VolumeIndex }}"
	_ = r.validateVolumeNameTemplate(p)
	if p.Status.HasCondition(NotValid) {
		t.Fatalf("expected no NotValid condition")
	}
}

func TestValidateNetworkNameTemplate_Invalid_SetsNotValidCondition(t *testing.T) {
	r := createFakeReconciler()
	p := &api.Plan{}
	p.Spec.NetworkNameTemplate = "Bad"
	_ = r.validateNetworkNameTemplate(p)
	if !p.Status.HasCondition(NotValid) {
		t.Fatalf("expected NotValid condition")
	}
}

func TestValidateNetworkNameTemplate_Valid_NoCondition(t *testing.T) {
	r := createFakeReconciler()
	p := &api.Plan{}
	p.Spec.NetworkNameTemplate = "{{ .NetworkName }}-{{ .NetworkIndex }}"
	_ = r.validateNetworkNameTemplate(p)
	if p.Status.HasCondition(NotValid) {
		t.Fatalf("expected no NotValid condition")
	}
}

func TestValidateWarmMigration_NotWarm_ReturnsNil(t *testing.T) {
	r := &Reconciler{}
	p := &api.Plan{}
	p.Spec.Warm = false
	if err := r.validateWarmMigration(p); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateWarmMigration_NoProvider_ReturnsNil(t *testing.T) {
	r := &Reconciler{}
	p := &api.Plan{}
	p.Spec.Warm = true
	p.Referenced.Provider.Source = nil
	if err := r.validateWarmMigration(p); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateWarmMigration_UnsupportedProvider_ReturnsError(t *testing.T) {
	r := &Reconciler{}
	p := &api.Plan{}
	p.Spec.Warm = true
	tp := api.ProviderType("nope")
	p.Referenced.Provider.Source = &api.Provider{Spec: api.ProviderSpec{Type: &tp}}
	if err := r.validateWarmMigration(p); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateNetworkMap_NotSet_SetsCondition(t *testing.T) {
	r := createFakeReconciler()
	p := &api.Plan{}
	p.Spec.Map.Network = core.ObjectReference{} // not set
	_ = r.validateNetworkMap(p)
	c := p.Status.FindCondition(NetRefNotValid)
	if c == nil || c.Reason != NotSet {
		t.Fatalf("expected NotSet, got %#v", c)
	}
}

func TestValidateNetworkMap_NotFound_SetsCondition(t *testing.T) {
	r := createFakeReconciler()
	p := &api.Plan{}
	p.Spec.Map.Network = core.ObjectReference{Namespace: "ns", Name: "missing"}
	_ = r.validateNetworkMap(p)
	c := p.Status.FindCondition(NetRefNotValid)
	if c == nil || c.Reason != NotFound {
		t.Fatalf("expected NotFound, got %#v", c)
	}
}

func TestValidateNetworkMap_NotReady_SetsNotReadyConditionAndReferencesMap(t *testing.T) {
	mp := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nm"}}
	// not Ready condition
	r := createFakeReconciler(mp)
	p := &api.Plan{}
	p.Spec.Map.Network = core.ObjectReference{Namespace: "ns", Name: "nm"}
	_ = r.validateNetworkMap(p)
	if p.Referenced.Map.Network == nil || p.Referenced.Map.Network.Name != "nm" {
		t.Fatalf("expected referenced map set")
	}
	if !p.Status.HasCondition(NetMapNotReady) {
		t.Fatalf("expected NetMapNotReady")
	}
}

func TestValidateNetworkMap_Ready_SetsReferenceWithoutNotReadyCondition(t *testing.T) {
	mp := &api.NetworkMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nm"}}
	mp.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True})
	r := createFakeReconciler(mp)
	p := &api.Plan{}
	p.Spec.Map.Network = core.ObjectReference{Namespace: "ns", Name: "nm"}
	_ = r.validateNetworkMap(p)
	if p.Referenced.Map.Network == nil || p.Referenced.Map.Network.Name != "nm" {
		t.Fatalf("expected referenced map set")
	}
	if p.Status.HasCondition(NetMapNotReady) {
		t.Fatalf("expected no NetMapNotReady")
	}
}

func TestValidateStorageMap_NotSet_SetsCondition(t *testing.T) {
	r := createFakeReconciler()
	p := &api.Plan{}
	p.Spec.Map.Storage = core.ObjectReference{} // not set
	_ = r.validateStorageMap(p)
	c := p.Status.FindCondition(DsRefNotValid)
	if c == nil || c.Reason != NotSet {
		t.Fatalf("expected NotSet, got %#v", c)
	}
}

func TestValidateStorageMap_NotFound_SetsCondition(t *testing.T) {
	r := createFakeReconciler()
	p := &api.Plan{}
	p.Spec.Map.Storage = core.ObjectReference{Namespace: "ns", Name: "missing"}
	_ = r.validateStorageMap(p)
	c := p.Status.FindCondition(DsRefNotValid)
	if c == nil || c.Reason != NotFound {
		t.Fatalf("expected NotFound, got %#v", c)
	}
}

func TestValidateStorageMap_NotReady_SetsNotReadyConditionAndReferencesMap(t *testing.T) {
	mp := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sm"}}
	r := createFakeReconciler(mp)
	p := &api.Plan{}
	p.Spec.Map.Storage = core.ObjectReference{Namespace: "ns", Name: "sm"}
	_ = r.validateStorageMap(p)
	if p.Referenced.Map.Storage == nil || p.Referenced.Map.Storage.Name != "sm" {
		t.Fatalf("expected referenced map set")
	}
	if !p.Status.HasCondition(DsMapNotReady) {
		t.Fatalf("expected DsMapNotReady")
	}
}

func TestValidateStorageMap_Ready_SetsReferenceWithoutNotReadyCondition(t *testing.T) {
	mp := &api.StorageMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sm"}}
	mp.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True})
	r := createFakeReconciler(mp)
	p := &api.Plan{}
	p.Spec.Map.Storage = core.ObjectReference{Namespace: "ns", Name: "sm"}
	_ = r.validateStorageMap(p)
	if p.Referenced.Map.Storage == nil || p.Referenced.Map.Storage.Name != "sm" {
		t.Fatalf("expected referenced map set")
	}
	if p.Status.HasCondition(DsMapNotReady) {
		t.Fatalf("expected no DsMapNotReady")
	}
}

func TestJobExceedsDeadline_NoStartTime_False(t *testing.T) {
	j := &batchv1.Job{}
	if jobExceedsDeadline(j) {
		t.Fatalf("expected false")
	}
}

func TestJobExceedsDeadline_WithinDeadline_False(t *testing.T) {
	old := settings.Settings.Migration.VddkJobActiveDeadline
	t.Cleanup(func() { settings.Settings.Migration.VddkJobActiveDeadline = old })
	settings.Settings.Migration.VddkJobActiveDeadline = 1000

	now := metav1.Now()
	j := &batchv1.Job{}
	j.Status.StartTime = &now
	if jobExceedsDeadline(j) {
		t.Fatalf("expected false")
	}
}

func TestJobExceedsDeadline_Exceeded_True(t *testing.T) {
	old := settings.Settings.Migration.VddkJobActiveDeadline
	t.Cleanup(func() { settings.Settings.Migration.VddkJobActiveDeadline = old })
	settings.Settings.Migration.VddkJobActiveDeadline = 1

	past := metav1.NewTime(time.Now().Add(-10 * time.Second))
	j := &batchv1.Job{}
	j.Status.StartTime = &past
	if !jobExceedsDeadline(j) {
		t.Fatalf("expected true")
	}
}
