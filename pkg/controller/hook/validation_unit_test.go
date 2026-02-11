package hook

import (
	"context"
	"encoding/base64"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/controller/base"
	"github.com/kubev2v/forklift/pkg/lib/condition"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconciler_validateImageAndPlaybook(t *testing.T) {
	r := &Reconciler{}

	// Invalid image => sets InvalidImage condition.
	h := &api.Hook{}
	h.Spec.Image = "not a valid image ref"
	h.Spec.Playbook = base64.StdEncoding.EncodeToString([]byte("ok"))
	if err := r.validate(h); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !h.Status.HasCondition(InvalidImage) {
		t.Fatalf("expected InvalidImage condition")
	}

	// Valid image but invalid playbook => sets InvalidPlaybook.
	h2 := &api.Hook{}
	h2.Spec.Image = "quay.io/konveyor/forklift:latest"
	h2.Spec.Playbook = "%%%not base64%%%"
	if err := r.validate(h2); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if h2.Status.HasCondition(InvalidImage) {
		t.Fatalf("did not expect InvalidImage for valid ref")
	}
	if !h2.Status.HasCondition(InvalidPlaybook) {
		t.Fatalf("expected InvalidPlaybook condition")
	}
}

// ---- Consolidated from controller_more_unit_test.go ----

func hookTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(corev1): %v", err)
	}
	if err := api.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(api): %v", err)
	}
	return s
}

func TestReconciler_Reconcile_NotFoundIsNoop(t *testing.T) {
	s := hookTestScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := Reconciler{
		Reconciler: base.Reconciler{
			Client:        cl,
			EventRecorder: record.NewFakeRecorder(50),
			Log:           logging.WithName("test-hook"),
		},
	}
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "missing"}})
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
}

func TestReconciler_Reconcile_ReadyWhenValid(t *testing.T) {
	s := hookTestScheme(t)
	h := &api.Hook{
		ObjectMeta: metav1.ObjectMeta{Name: "h1", Namespace: "ns"},
		Spec: api.HookSpec{
			Image:    "quay.io/test/image:latest",
			Playbook: base64.StdEncoding.EncodeToString([]byte("echo ok")),
		},
	}
	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&api.Hook{}).
		WithRuntimeObjects(h).
		Build()
	r := Reconciler{
		Reconciler: base.Reconciler{
			Client:        cl,
			EventRecorder: record.NewFakeRecorder(50),
			Log:           logging.WithName("test-hook"),
		},
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "h1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	updated := &api.Hook{}
	if err := cl.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "h1"}, updated); err != nil {
		t.Fatalf("get updated hook: %v", err)
	}
	if !updated.Status.HasCondition(condition.Ready) {
		t.Fatalf("expected Ready condition set, got: %#v", updated.Status.Conditions)
	}
}

func TestReconciler_Reconcile_InvalidPlaybook_SetsConditionNotReady(t *testing.T) {
	s := hookTestScheme(t)
	h := &api.Hook{
		ObjectMeta: metav1.ObjectMeta{Name: "h1", Namespace: "ns"},
		Spec: api.HookSpec{
			Image:    "quay.io/test/image:latest",
			Playbook: "not-base64",
		},
	}
	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&api.Hook{}).
		WithRuntimeObjects(h).
		Build()
	r := Reconciler{
		Reconciler: base.Reconciler{
			Client:        cl,
			EventRecorder: record.NewFakeRecorder(50),
			Log:           logging.WithName("test-hook"),
		},
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "h1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	updated := &api.Hook{}
	if err := cl.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "h1"}, updated); err != nil {
		t.Fatalf("get updated hook: %v", err)
	}
	if !updated.Status.HasCondition(InvalidPlaybook) {
		t.Fatalf("expected InvalidPlaybook condition set, got: %#v", updated.Status.Conditions)
	}
	if updated.Status.HasCondition(condition.Ready) {
		t.Fatalf("did not expect Ready condition")
	}
}
