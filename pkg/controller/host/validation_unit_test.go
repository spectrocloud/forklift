package host

import (
	"context"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/controller/base"
	"github.com/kubev2v/forklift/pkg/controller/validation"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconciler_validateProvider_NotSetAndWrongType(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)

	// NotSet: provider ref empty => ProviderNotValid condition set, referenced nil.
	r := &Reconciler{}
	r.Log = logging.WithName("test")
	r.Client = fake.NewClientBuilder().WithScheme(s).Build()

	h := &api.Host{}
	if err := r.validateProvider(h); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !h.Status.HasCondition("ProviderNotValid") {
		t.Fatalf("expected ProviderNotValid condition")
	}

	// Wrong type: provider exists but isn't VSphere => TypeNotValid.
	tp := api.OpenShift
	p := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec:       api.ProviderSpec{Type: &tp},
	}
	// Mark provider Ready so validation.Referenced gets populated.
	p.Status.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True, Category: libcnd.Required})

	r2 := &Reconciler{}
	r2.Log = logging.WithName("test")
	r2.Client = fake.NewClientBuilder().WithScheme(s).WithObjects(p).Build()

	h2 := &api.Host{}
	h2.Spec.Provider = core.ObjectReference{Namespace: "ns", Name: "p"}
	if err := r2.validateProvider(h2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !h2.Status.HasCondition(TypeNotValid) {
		t.Fatalf("expected TypeNotValid condition")
	}
}

func TestReconciler_validateRefAndIp_NotSet(t *testing.T) {
	r := &Reconciler{}

	h := &api.Host{}
	if err := r.validateRef(h); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !h.Status.HasCondition(RefNotValid) {
		t.Fatalf("expected RefNotValid")
	}

	h2 := &api.Host{}
	h2.Spec.IpAddress = ""
	if err := r.validateIp(h2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !h2.Status.HasCondition(IpNotValid) {
		t.Fatalf("expected IpNotValid")
	}
}

func TestReconciler_validateSecret_NotSetNotFoundAndMissingKeys(t *testing.T) {
	s := runtime.NewScheme()
	_ = api.SchemeBuilder.AddToScheme(s)
	_ = core.AddToScheme(s)

	// NotSet: no ref => condition set.
	r := &Reconciler{}
	r.Client = fake.NewClientBuilder().WithScheme(s).Build()
	h := &api.Host{}
	if err := r.validateSecret(h); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !h.Status.HasCondition(SecretNotValid) {
		t.Fatalf("expected SecretNotValid")
	}

	// NotFound: ref set but secret missing.
	h2 := &api.Host{}
	h2.Spec.Secret = core.ObjectReference{Namespace: "ns", Name: "missing"}
	if err := r.validateSecret(h2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !h2.Status.HasCondition(SecretNotValid) {
		t.Fatalf("expected SecretNotValid")
	}

	// Missing keys (vsphere): provider=vsphere, secret exists but missing user/password.
	tp := api.VSphere
	p := &api.Provider{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec:       api.ProviderSpec{Type: &tp},
	}
	sec := &core.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "s"},
		Data:       map[string][]byte{api.Insecure: []byte("true")},
	}
	r3 := &Reconciler{}
	r3.Client = fake.NewClientBuilder().WithScheme(s).WithObjects(p, sec).Build()

	h3 := &api.Host{}
	h3.Spec.Secret = core.ObjectReference{Namespace: "ns", Name: "s"}
	h3.Referenced.Provider.Source = p
	if err := r3.validateSecret(h3); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !h3.Status.HasCondition(SecretNotValid) {
		t.Fatalf("expected SecretNotValid")
	}
}

// ---- Consolidated from controller_more_unit_test.go ----

func hostTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := core.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(corev1): %v", err)
	}
	if err := api.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(api): %v", err)
	}
	return s
}

func TestReconciler_Reconcile_NotFoundIsNoop(t *testing.T) {
	s := hostTestScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := Reconciler{
		Reconciler: base.Reconciler{
			Client:        cl,
			EventRecorder: record.NewFakeRecorder(100),
			Log:           logging.WithName("test-host"),
		},
	}
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "missing"}})
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
}

func TestReconciler_Reconcile_SetsValidationConditionsAndUpdatesStatus(t *testing.T) {
	s := hostTestScheme(t)
	host := &api.Host{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "h1",
			Namespace: "ns",
		},
		Spec: api.HostSpec{
			// Intentionally leave Provider, Ref, IpAddress, Secret unset to trigger validation conditions.
		},
	}
	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&api.Host{}).
		WithRuntimeObjects(host).
		Build()
	r := Reconciler{
		Reconciler: base.Reconciler{
			Client:        cl,
			EventRecorder: record.NewFakeRecorder(100),
			Log:           logging.WithName("test-host"),
		},
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "h1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	updated := &api.Host{}
	if err := cl.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "h1"}, updated); err != nil {
		t.Fatalf("failed to get updated host: %v", err)
	}

	// Provider validation: missing provider ref => ProviderNotValid condition.
	if !updated.Status.HasCondition(validation.ProviderNotValid) {
		t.Fatalf("expected ProviderNotValid condition set, got: %#v", updated.Status.Conditions)
	}
	// Host-specific validations.
	if !updated.Status.HasCondition(RefNotValid) {
		t.Fatalf("expected RefNotValid condition set")
	}
	if !updated.Status.HasCondition(IpNotValid) {
		t.Fatalf("expected IpNotValid condition set")
	}
	if !updated.Status.HasCondition(SecretNotValid) {
		t.Fatalf("expected SecretNotValid condition set")
	}
	if !updated.Status.HasCondition(Validated) {
		t.Fatalf("expected Validated condition set")
	}
	// Should not mark Ready when blockers exist.
	if updated.Status.HasCondition("Ready") {
		t.Fatalf("did not expect Ready condition when blockers exist")
	}
}
