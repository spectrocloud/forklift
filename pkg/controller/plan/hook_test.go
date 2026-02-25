package plan

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	web "github.com/kubev2v/forklift/pkg/controller/provider/web"
	webbase "github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeWebClient struct {
	workloadFn func(ref *webbase.Ref) (interface{}, error)
}

func (f *fakeWebClient) Finder() web.Finder { return nil }
func (f *fakeWebClient) Get(resource interface{}, id string) error {
	return errors.New("not implemented")
}
func (f *fakeWebClient) List(list interface{}, param ...web.Param) error {
	return errors.New("not implemented")
}
func (f *fakeWebClient) Watch(resource interface{}, h web.EventHandler) (*web.Watch, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeWebClient) Find(resource interface{}, ref webbase.Ref) error {
	return errors.New("not implemented")
}
func (f *fakeWebClient) VM(ref *webbase.Ref) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeWebClient) Network(ref *webbase.Ref) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeWebClient) Storage(ref *webbase.Ref) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeWebClient) Host(ref *webbase.Ref) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeWebClient) Workload(ref *webbase.Ref) (interface{}, error) {
	if f.workloadFn != nil {
		return f.workloadFn(ref)
	}
	return map[string]any{"x": "y"}, nil
}

func newHookRunnerHarness(t *testing.T) (*HookRunner, client.Client, *api.Plan, *api.Migration, *planapi.VMStatus, *api.Hook) {
	t.Helper()
	// HookRunner.template uses resource.MustParse on these settings; ensure they are non-empty.
	oldReqCPU := Settings.Migration.HooksContainerRequestsCpu
	oldReqMem := Settings.Migration.HooksContainerRequestsMemory
	oldLimCPU := Settings.Migration.HooksContainerLimitsCpu
	oldLimMem := Settings.Migration.HooksContainerLimitsMemory
	t.Cleanup(func() {
		Settings.Migration.HooksContainerRequestsCpu = oldReqCPU
		Settings.Migration.HooksContainerRequestsMemory = oldReqMem
		Settings.Migration.HooksContainerLimitsCpu = oldLimCPU
		Settings.Migration.HooksContainerLimitsMemory = oldLimMem
	})
	if Settings.Migration.HooksContainerRequestsCpu == "" {
		Settings.Migration.HooksContainerRequestsCpu = "10m"
	}
	if Settings.Migration.HooksContainerRequestsMemory == "" {
		Settings.Migration.HooksContainerRequestsMemory = "16Mi"
	}
	if Settings.Migration.HooksContainerLimitsCpu == "" {
		Settings.Migration.HooksContainerLimitsCpu = "100m"
	}
	if Settings.Migration.HooksContainerLimitsMemory == "" {
		Settings.Migration.HooksContainerLimitsMemory = "64Mi"
	}

	// HookRunner uses controller-runtime's SetOwnerReference with the *global* scheme.Scheme.
	// Ensure forklift API types are registered there so OwnerReference can be set.
	_ = api.SchemeBuilder.AddToScheme(kubescheme.Scheme)

	s := runtime.NewScheme()
	_ = core.AddToScheme(s)
	_ = batch.AddToScheme(s)
	_ = api.SchemeBuilder.AddToScheme(s)

	cl := fake.NewClientBuilder().WithScheme(s).Build()

	plan := &api.Plan{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p", UID: "plan-uid"}}
	mig := &api.Migration{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "m", UID: "mig-uid"}}
	hook := &api.Hook{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "h"}}
	hook.Spec.Image = "img"

	vm := &planapi.VMStatus{}
	vm.ID = "vm-1"
	vm.Ref = ref.Ref{ID: "vm-1"}
	vm.Phase = api.PhasePreHook
	vm.Pipeline = []*planapi.Step{{Task: planapi.Task{Name: api.PhasePreHook}}}

	ctx := &plancontext.Context{
		Client:    cl,
		Plan:      plan,
		Migration: mig,
		Log:       logging.WithName("t"),
	}
	ctx.Source.Inventory = &fakeWebClient{}
	ctx.Hooks = []*api.Hook{hook}

	r := &HookRunner{Context: ctx, hook: hook}
	return r, cl, plan, mig, vm, hook
}

func TestHookRunner_LabelsContainExpectedKeys(t *testing.T) {
	r, _, _, _, vm, _ := newHookRunnerHarness(t)
	r.vm = vm
	lbl := r.labels()
	if lbl[kPlan] != "plan-uid" || lbl[kMigration] != "mig-uid" || lbl[kVM] != "vm-1" || lbl[kStep] != api.PhasePreHook {
		t.Fatalf("unexpected labels: %#v", lbl)
	}
}

func TestHookRunner_LabelsReflectCurrentVMPhase(t *testing.T) {
	r, _, _, _, vm, _ := newHookRunnerHarness(t)
	r.vm = vm
	vm.Phase = api.PhasePostHook
	lbl := r.labels()
	if lbl[kStep] != api.PhasePostHook {
		t.Fatalf("expected %s got %s", api.PhasePostHook, lbl[kStep])
	}
}

func TestHookRunner_Playbook_Empty_ReturnsEmpty(t *testing.T) {
	r, _, _, _, _, hook := newHookRunnerHarness(t)
	hook.Spec.Playbook = ""
	got, err := r.playbook()
	if err != nil || got != "" {
		t.Fatalf("expected empty nil got %q %v", got, err)
	}
}

func TestHookRunner_Playbook_EmptyDoesNotErrorEvenWithWhitespace(t *testing.T) {
	r, _, _, _, _, hook := newHookRunnerHarness(t)
	hook.Spec.Playbook = ""
	got, err := r.playbook()
	if err != nil || got != "" {
		t.Fatalf("expected empty nil got %q %v", got, err)
	}
}

func TestHookRunner_Playbook_InvalidBase64_ReturnsError(t *testing.T) {
	r, _, _, _, _, hook := newHookRunnerHarness(t)
	hook.Spec.Playbook = "!!!"
	_, err := r.playbook()
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestHookRunner_Playbook_ValidBase64EmptyPayload_ReturnsEmptyString(t *testing.T) {
	r, _, _, _, _, hook := newHookRunnerHarness(t)
	hook.Spec.Playbook = base64.StdEncoding.EncodeToString([]byte(""))
	got, err := r.playbook()
	if err != nil || got != "" {
		t.Fatalf("expected empty nil got %q %v", got, err)
	}
}

func TestHookRunner_Playbook_DecodesBase64(t *testing.T) {
	r, _, _, _, _, hook := newHookRunnerHarness(t)
	hook.Spec.Playbook = base64.StdEncoding.EncodeToString([]byte("hello"))
	got, err := r.playbook()
	if err != nil || got != "hello" {
		t.Fatalf("expected hello nil got %q %v", got, err)
	}
}

func TestHookRunner_Plan_YamlNotEmpty(t *testing.T) {
	r, _, plan, _, _, _ := newHookRunnerHarness(t)
	plan.Spec.TargetNamespace = "tns"
	got, err := r.plan()
	if err != nil || got == "" {
		t.Fatalf("expected yaml, got %q %v", got, err)
	}
}

func TestHookRunner_Plan_YamlChangesWhenSpecChanges(t *testing.T) {
	r, _, plan, _, _, _ := newHookRunnerHarness(t)
	plan.Spec.TargetNamespace = "a"
	a, _ := r.plan()
	plan.Spec.TargetNamespace = "b"
	b, _ := r.plan()
	if a == b {
		t.Fatalf("expected different yaml")
	}
}

func TestHookRunner_Workload_UsesInventory(t *testing.T) {
	r, _, _, _, vm, _ := newHookRunnerHarness(t)
	r.vm = vm
	r.Source.Inventory = &fakeWebClient{workloadFn: func(ref *webbase.Ref) (interface{}, error) {
		if ref == nil || ref.ID != "vm-1" {
			t.Fatalf("unexpected ref: %#v", ref)
		}
		return map[string]any{"a": "b"}, nil
	}}
	got, err := r.workload()
	if err != nil || got == "" {
		t.Fatalf("expected yaml, got %q %v", got, err)
	}
}

func TestHookRunner_Workload_InventoryError(t *testing.T) {
	r, _, _, _, vm, _ := newHookRunnerHarness(t)
	r.vm = vm
	r.Source.Inventory = &fakeWebClient{workloadFn: func(ref *webbase.Ref) (interface{}, error) {
		return nil, errors.New("boom")
	}}
	_, err := r.workload()
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestHookRunner_ConfigMap_IncludesWorkloadPlanPlaybookKeys(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	mp, err := r.configMap()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, k := range []string{"workload.yml", "plan.yml", "playbook.yml"} {
		if _, ok := mp.Data[k]; !ok {
			t.Fatalf("expected key %s", k)
		}
	}
}

func TestHookRunner_Template_Defaults(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	mp := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	pt := r.template(mp)
	if pt.Spec.RestartPolicy != core.RestartPolicyNever {
		t.Fatalf("expected never")
	}
	if pt.Spec.Containers[0].Image != "img" {
		t.Fatalf("expected image img got %s", pt.Spec.Containers[0].Image)
	}
	if pt.Spec.Volumes[0].ConfigMap == nil || pt.Spec.Volumes[0].ConfigMap.Name != "cm" {
		t.Fatalf("unexpected volumes: %#v", pt.Spec.Volumes)
	}
}

func TestHookRunner_Template_NoDeadlineLeavesNil(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	hook.Spec.Deadline = 0
	r.hook = hook
	mp := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	pt := r.template(mp)
	if pt.Spec.ActiveDeadlineSeconds != nil {
		t.Fatalf("expected nil deadline")
	}
}

func TestHookRunner_Template_EmptyServiceAccountLeavesEmpty(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	hook.Spec.ServiceAccount = ""
	r.hook = hook
	mp := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	pt := r.template(mp)
	if pt.Spec.ServiceAccountName != "" {
		t.Fatalf("expected empty service account")
	}
}

func TestHookRunner_Template_EmptyPlaybookLeavesNoCommand(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	hook.Spec.Playbook = ""
	r.hook = hook
	mp := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	pt := r.template(mp)
	if len(pt.Spec.Containers[0].Command) != 0 {
		t.Fatalf("expected no command")
	}
}

func TestHookRunner_Template_DeadlineSetsActiveDeadlineSeconds(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	hook.Spec.Deadline = 123
	r.hook = hook
	mp := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	pt := r.template(mp)
	if pt.Spec.ActiveDeadlineSeconds == nil || *pt.Spec.ActiveDeadlineSeconds != 123 {
		t.Fatalf("expected 123 got %#v", pt.Spec.ActiveDeadlineSeconds)
	}
}

func TestHookRunner_Template_ServiceAccountSet(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	hook.Spec.ServiceAccount = "sa1"
	r.hook = hook
	mp := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	pt := r.template(mp)
	if pt.Spec.ServiceAccountName != "sa1" {
		t.Fatalf("expected sa1 got %s", pt.Spec.ServiceAccountName)
	}
}

func TestHookRunner_Template_PlaybookCommandHasEntrypoint(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	hook.Spec.Playbook = base64.StdEncoding.EncodeToString([]byte("x"))
	r.hook = hook
	mp := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	pt := r.template(mp)
	if len(pt.Spec.Containers[0].Command) < 1 || pt.Spec.Containers[0].Command[0] != "/bin/entrypoint" {
		t.Fatalf("unexpected command: %#v", pt.Spec.Containers[0].Command)
	}
}

func TestHookRunner_Template_PlaybookCommandIncludesPlaybookPath(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	hook.Spec.Playbook = base64.StdEncoding.EncodeToString([]byte("x"))
	r.hook = hook
	mp := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	pt := r.template(mp)
	found := false
	for _, a := range pt.Spec.Containers[0].Command {
		if a == "/tmp/hook/playbook.yml" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected playbook path in command: %#v", pt.Spec.Containers[0].Command)
	}
}

func TestHookRunner_Template_PlaybookSetsCommand(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	hook.Spec.Playbook = base64.StdEncoding.EncodeToString([]byte("x"))
	r.hook = hook
	mp := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	pt := r.template(mp)
	if len(pt.Spec.Containers[0].Command) == 0 {
		t.Fatalf("expected command set")
	}
}

func TestHookRunner_Template_SetsVolumeMountPath(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	mp := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	pt := r.template(mp)
	if pt.Spec.Containers[0].VolumeMounts[0].MountPath != "/tmp/hook" {
		t.Fatalf("unexpected mount path: %s", pt.Spec.Containers[0].VolumeMounts[0].MountPath)
	}
}

func TestHookRunner_Template_ConfigMapVolumeUsesName(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	mp := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cmX"}}
	pt := r.template(mp)
	if pt.Spec.Volumes[0].ConfigMap == nil || pt.Spec.Volumes[0].ConfigMap.Name != "cmX" {
		t.Fatalf("unexpected configmap volume: %#v", pt.Spec.Volumes[0])
	}
}

func TestHookRunner_ConfigMap_BuildsDataKeys(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	mp, err := r.configMap()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if mp.Data["workload.yml"] == "" || mp.Data["plan.yml"] == "" {
		t.Fatalf("expected workload/plan data")
	}
	// playbook.yml present even when empty.
	if _, ok := mp.Data["playbook.yml"]; !ok {
		t.Fatalf("expected playbook.yml key")
	}
}

func TestHookRunner_ConfigMap_GenerateNameLowercaseAndIncludesIDs(t *testing.T) {
	r, _, plan, _, vm, hook := newHookRunnerHarness(t)
	plan.Name = "MyPlan"
	vm.ID = "VMID"
	vm.Phase = "STEP"
	r.vm = vm
	r.hook = hook
	mp, err := r.configMap()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if mp.GenerateName != "myplan-vmid-step-" {
		t.Fatalf("unexpected generateName: %q", mp.GenerateName)
	}
}

func TestHookRunner_ConfigMap_LabelsMatchRunnerLabels(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	mp, err := r.configMap()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := r.labels()
	for k, v := range want {
		if mp.Labels[k] != v {
			t.Fatalf("label %s expected %s got %s", k, v, mp.Labels[k])
		}
	}
}

func TestHookRunner_ConfigMap_PlaybookKeyPresentWhenEmpty(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	hook.Spec.Playbook = ""
	r.hook = hook
	mp, err := r.configMap()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := mp.Data["playbook.yml"]; !ok {
		t.Fatalf("expected playbook.yml key")
	}
}

func TestHookRunner_ConfigMap_PlaybookKeyContainsDecoded(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	hook.Spec.Playbook = base64.StdEncoding.EncodeToString([]byte("PB"))
	r.hook = hook
	mp, err := r.configMap()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if mp.Data["playbook.yml"] != "PB" {
		t.Fatalf("expected PB got %q", mp.Data["playbook.yml"])
	}
}

func TestHookRunner_ConfigMap_PlaybookInvalidBase64ReturnsError(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	hook.Spec.Playbook = "!!!"
	r.hook = hook
	_, err := r.configMap()
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestHookRunner_Workload_MarshalPanicsOnUnsupportedType(t *testing.T) {
	r, _, _, _, vm, _ := newHookRunnerHarness(t)
	r.vm = vm
	r.Source.Inventory = &fakeWebClient{workloadFn: func(ref *webbase.Ref) (interface{}, error) {
		return map[string]any{"f": func() {}}, nil
	}}
	defer func() {
		if rec := recover(); rec == nil {
			t.Fatalf("expected panic")
		}
	}()
	_, _ = r.workload()
}

func TestHookRunner_EnsureConfigMap_CreatesWhenMissing(t *testing.T) {
	r, cl, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	mp, err := r.ensureConfigMap()
	if err != nil || mp == nil {
		t.Fatalf("expected configmap, got %v %#v", err, mp)
	}
	// verify exists
	got := &core.ConfigMap{}
	if gErr := cl.Get(context.TODO(), client.ObjectKey{Namespace: "ns", Name: mp.Name}, got); gErr != nil {
		t.Fatalf("expected get ok: %v", gErr)
	}
}

func TestHookRunner_EnsureConfigMap_FindsExisting(t *testing.T) {
	r, cl, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	// create one
	first, err := r.ensureConfigMap()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// ensure again should find (same label selector)
	second, err := r.ensureConfigMap()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if first.Name != second.Name {
		t.Fatalf("expected same configmap")
	}
	// sanity: client can list them
	list := &core.ConfigMapList{}
	_ = cl.List(context.TODO(), list)
}

func TestHookRunner_EnsureConfigMap_UsesLabelsSelector(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	mp, err := r.ensureConfigMap()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	lbl := r.labels()
	for k, v := range lbl {
		if mp.Labels[k] != v {
			t.Fatalf("label %s expected %s got %s", k, v, mp.Labels[k])
		}
	}
}

func TestHookRunner_EnsureJob_CreatesWhenMissing(t *testing.T) {
	r, cl, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	job, err := r.ensureJob()
	if err != nil || job == nil {
		t.Fatalf("expected job, got %v %#v", err, job)
	}
	got := &batch.Job{}
	if gErr := cl.Get(context.TODO(), client.ObjectKey{Namespace: "ns", Name: job.Name}, got); gErr != nil {
		t.Fatalf("expected get ok: %v", gErr)
	}
}

func TestHookRunner_EnsureJob_CreatesConfigMapFirst(t *testing.T) {
	r, cl, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	_, err := r.ensureJob()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	cms := &core.ConfigMapList{}
	_ = cl.List(context.TODO(), cms)
	if len(cms.Items) == 0 {
		t.Fatalf("expected configmap created")
	}
}

func TestHookRunner_EnsureJob_SetsConfigMapOwnerReferenceToJob(t *testing.T) {
	r, cl, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	job, err := r.ensureJob()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// find configmap by listing and checking labels
	list := &core.ConfigMapList{}
	if lErr := cl.List(context.TODO(), list); lErr != nil {
		t.Fatalf("list: %v", lErr)
	}
	if len(list.Items) == 0 {
		t.Fatalf("expected configmap created")
	}
	mp := &list.Items[0]
	if len(mp.OwnerReferences) == 0 || mp.OwnerReferences[0].Name != job.Name {
		t.Fatalf("expected ownerRef to job, got %#v", mp.OwnerReferences)
	}
}

func TestHookRunner_EnsureJob_FindsExisting(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	first, err := r.ensureJob()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	second, err := r.ensureJob()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if first.Name != second.Name {
		t.Fatalf("expected same job")
	}
}

func TestHookRunner_EnsureJob_FindsExistingAndStillUpdatesOwnerReference(t *testing.T) {
	r, cl, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	// first creates both
	_, err := r.ensureJob()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// second finds job and updates configmap owner ref too
	job2, err := r.ensureJob()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	list := &core.ConfigMapList{}
	_ = cl.List(context.TODO(), list)
	mp := &list.Items[0]
	if len(mp.OwnerReferences) == 0 || mp.OwnerReferences[0].Name != job2.Name {
		t.Fatalf("expected ownerRef to %s, got %#v", job2.Name, mp.OwnerReferences)
	}
}

func TestHookRunner_Job_GenerateNameLowercaseAndIncludesIDs(t *testing.T) {
	r, _, plan, _, vm, hook := newHookRunnerHarness(t)
	plan.Name = "MyPlan"
	vm.ID = "VMID"
	vm.Phase = "STEP"
	r.vm = vm
	r.hook = hook
	cm := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	job, err := r.job(cm)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if job.GenerateName == "" || job.GenerateName != "myplan-vmid-step-" {
		t.Fatalf("unexpected generateName: %q", job.GenerateName)
	}
}

func TestHookRunner_Job_BackoffLimitIsOne(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	cm := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	job, err := r.job(cm)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 1 {
		t.Fatalf("expected backoff 1 got %#v", job.Spec.BackoffLimit)
	}
}

func TestHookRunner_Job_NamespaceMatchesPlan(t *testing.T) {
	r, _, plan, _, vm, hook := newHookRunnerHarness(t)
	plan.Namespace = "ns2"
	r.vm = vm
	r.hook = hook
	cm := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	job, err := r.job(cm)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if job.Namespace != "ns2" {
		t.Fatalf("expected ns2 got %s", job.Namespace)
	}
}

func TestHookRunner_Job_LabelsMatchRunnerLabels(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.vm = vm
	r.hook = hook
	cm := &core.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm"}}
	job, err := r.job(cm)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := r.labels()
	for k, v := range want {
		if job.Labels[k] != v {
			t.Fatalf("label %s expected %s got %s", k, v, job.Labels[k])
		}
	}
}

func TestHookRunner_Run_NoHookForPhase_NoError(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.hook = hook
	r.vm = vm
	// no hooks configured for this vm
	vm.Hooks = nil
	if err := r.Run(vm); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestHookRunner_Run_NoHookForPhase_DoesNotSetStepError(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.hook = hook
	r.vm = vm
	vm.Hooks = nil
	if err := r.Run(vm); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	step, _ := vm.FindStep(api.PhasePreHook)
	if step.Error != nil {
		t.Fatalf("expected no error")
	}
}

func TestHookRunner_Run_EnsureJobErrorFromConfigMapPropagates(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.hook = hook
	r.vm = vm
	vm.Hooks = []planapi.HookRef{{Step: api.PhasePreHook, Hook: core.ObjectReference{Name: "h", Namespace: "ns"}}}
	// force configMap() to fail via invalid playbook base64
	hook.Spec.Playbook = "!!!"
	if err := r.Run(vm); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHookRunner_Run_EnsureJobErrorFromWorkloadPropagates(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.hook = hook
	r.vm = vm
	vm.Hooks = []planapi.HookRef{{Step: api.PhasePreHook, Hook: core.ObjectReference{Name: "h", Namespace: "ns"}}}
	r.Source.Inventory = &fakeWebClient{workloadFn: func(ref *webbase.Ref) (interface{}, error) {
		return nil, errors.New("boom")
	}}
	if err := r.Run(vm); err == nil {
		t.Fatalf("expected error")
	}
}

func TestFakeWebClient_ImplementsWebClient(t *testing.T) {
	var _ web.Client = &fakeWebClient{}
}

func TestHookRunner_Run_ErrWhenStepNotFound(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.hook = hook
	vm.Pipeline = nil
	if err := r.Run(vm); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHookRunner_Run_HookNotFoundSetsStepError(t *testing.T) {
	r, _, _, _, vm, hook := newHookRunnerHarness(t)
	r.hook = hook
	vm.Hooks = []planapi.HookRef{{Step: api.PhasePreHook, Hook: core.ObjectReference{Name: "missing", Namespace: "ns"}}}
	// remove hook from ctx
	r.Context.Hooks = nil
	if err := r.Run(vm); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	step, _ := vm.FindStep(api.PhasePreHook)
	if step.Error == nil || len(step.Error.Reasons) == 0 {
		t.Fatalf("expected step error set")
	}
}

func TestHookRunner_Run_JobSucceededMarksCompletedAndProgress(t *testing.T) {
	r, cl, _, _, vm, hook := newHookRunnerHarness(t)
	r.hook = hook
	r.vm = vm
	// ensure hook ref exists
	vm.Hooks = []planapi.HookRef{{Step: api.PhasePreHook, Hook: core.ObjectReference{Name: "h", Namespace: "ns"}}}

	// create job with succeeded=1 matching labels so ensureJob finds it.
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "j1",
			Labels:    r.labels(),
		},
		Status: batch.JobStatus{Succeeded: 1},
	}
	if err := cl.Create(context.TODO(), job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.Run(vm); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	step, _ := vm.FindStep(api.PhasePreHook)
	if step.Progress.Completed != 1 {
		t.Fatalf("expected progress 1 got %d", step.Progress.Completed)
	}
	if !step.MarkedCompleted() {
		t.Fatalf("expected step completed")
	}
}

func TestHookRunner_Run_JobFailedConditionAddsError(t *testing.T) {
	r, cl, _, _, vm, hook := newHookRunnerHarness(t)
	r.hook = hook
	r.vm = vm
	vm.Hooks = []planapi.HookRef{{Step: api.PhasePreHook, Hook: core.ObjectReference{Name: "h", Namespace: "ns"}}}
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "j1",
			Labels:    r.labels(),
		},
		Status: batch.JobStatus{
			Conditions: []batch.JobCondition{{Type: batch.JobFailed, Status: core.ConditionTrue, Message: "nope"}},
		},
	}
	if err := cl.Create(context.TODO(), job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.Run(vm); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	step, _ := vm.FindStep(api.PhasePreHook)
	if step.Error == nil || len(step.Error.Reasons) == 0 {
		t.Fatalf("expected error")
	}
	if !step.MarkedCompleted() {
		t.Fatalf("expected completed")
	}
}

func TestHookRunner_Run_RetryLimitExceededAddsError(t *testing.T) {
	r, cl, _, _, vm, hook := newHookRunnerHarness(t)
	r.hook = hook
	r.vm = vm
	vm.Hooks = []planapi.HookRef{{Step: api.PhasePreHook, Hook: core.ObjectReference{Name: "h", Namespace: "ns"}}}

	old := Settings.Migration.HookRetry
	t.Cleanup(func() { Settings.Migration.HookRetry = old })
	Settings.Migration.HookRetry = 0

	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "j1",
			Labels:    r.labels(),
		},
		Status: batch.JobStatus{Failed: 1},
	}
	if err := cl.Create(context.TODO(), job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.Run(vm); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	step, _ := vm.FindStep(api.PhasePreHook)
	if step.Error == nil || len(step.Error.Reasons) == 0 {
		t.Fatalf("expected error")
	}
	if !step.MarkedCompleted() {
		t.Fatalf("expected completed")
	}
}
