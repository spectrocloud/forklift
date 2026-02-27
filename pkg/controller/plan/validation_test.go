package plan

import (
	"strconv"
	"testing"
	"time"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/provider"
	"github.com/kubev2v/forklift/pkg/controller/base"
	libcnd "github.com/kubev2v/forklift/pkg/lib/condition"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	"github.com/kubev2v/forklift/pkg/settings"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	discovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
	fakeClient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	sourceNamespace  = "source-namespace"
	destNamespace    = "destination-namespace"
	testNamespace    = "test-namespace"
	sourceName       = "source"
	destName         = "destination"
	sourceSecretName = "source-secret"
	testPlanName     = "test-plan"
	tokenKey         = "token"
	tokenValue       = "token"
	insecureSkipKey  = "inscureSkipVerify"
)

var (
	planValidationLog = logging.WithName("planValidation")
)

var _ = ginkgo.Describe("Plan Validations", func() {
	var (
		fakeClientSet *fake.Clientset
		reconciler    *Reconciler
	)

	ginkgo.BeforeEach(func() {
		reconciler = &Reconciler{
			base.Reconciler{},
		}
		fakeClientSet = fake.NewSimpleClientset()
	})

	ginkgo.Describe("validateOCPVersion", func() {
		ginkgo.DescribeTable("should validate OpenShift version correctly",
			func(major, minor string, shouldError bool) {
				fakeDiscovery, ok := fakeClientSet.Discovery().(*discovery.FakeDiscovery)
				gomega.Expect(ok).To(gomega.BeTrue())
				fakeDiscovery.FakedServerVersion = &version.Info{
					Major: major, Minor: minor,
				}

				err := reconciler.checkOCPVersion(fakeClientSet)

				if shouldError {
					gomega.Expect(err).To(gomega.HaveOccurred())
				} else {
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
				}
			},

			// Directly declare entries here
			ginkgo.Entry("when the OpenShift version is supported", "1", "26", false),
			ginkgo.Entry("when the OpenShift version is not supported", "1", "25", true),
		)
	})

	ginkgo.Describe("validate", func() {
		ginkgo.It("Should setup secret when source is not local cluster", func() {
			secret := createSecret(sourceSecretName, sourceNamespace, false)
			source := createProvider(sourceName, sourceNamespace, "https://source", api.OpenShift, &core.ObjectReference{Name: sourceSecretName, Namespace: sourceNamespace})
			destination := createProvider(destName, destNamespace, "", api.OpenShift, &core.ObjectReference{})
			plan := createPlan(testPlanName, testNamespace, source, destination)
		source.Status.Conditions.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True})
		destination.Status.Conditions.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True})

		reconciler = createFakeReconciler(secret, plan, source, destination)
		err := reconciler.ensureSecretForProvider(plan)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// secret should be set on plan.Referenced.Secret
		gomega.Expect(plan.Referenced.Secret).NotTo(gomega.BeNil())
	})

	ginkgo.It("Should not setup secret when source is local cluster", func() {
		secret := createSecret(sourceSecretName, sourceNamespace, false)
		source := createProvider(sourceName, sourceNamespace, "", api.OpenShift, &core.ObjectReference{Name: sourceSecretName, Namespace: sourceNamespace})
		destination := createProvider(destName, destNamespace, "https://destination", api.OpenShift, &core.ObjectReference{})
		plan := createPlan(testPlanName, testNamespace, source, destination)
		source.Status.Conditions.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True})
		destination.Status.Conditions.SetCondition(libcnd.Condition{Type: libcnd.Ready, Status: libcnd.True})

			reconciler = createFakeReconciler(secret, plan, source, destination)
			err := reconciler.ensureSecretForProvider(plan)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// secret should NOT be set on plan.Referenced.Secret
			gomega.Expect(plan.Referenced.Secret).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("validatePVCNameTemplate", func() {
		var reconciler *Reconciler

		ginkgo.BeforeEach(func() {
			reconciler = &Reconciler{
				Reconciler: base.Reconciler{
					Log: planValidationLog,
				},
			}
		})

		source := createProvider(sourceName, sourceNamespace, "", api.OpenShift, &core.ObjectReference{Name: sourceSecretName, Namespace: sourceNamespace})
		destination := createProvider(destName, destNamespace, "https://destination", api.OpenShift, &core.ObjectReference{})

		ginkgo.DescribeTable("should validate a plan correctly",
			func(template string, shouldBeValid bool) {
				plan := createPlan(testPlanName, testNamespace, source, destination)
				plan.Spec.PVCNameTemplate = template

				err := reconciler.validatePVCNameTemplate(plan)
				if err != nil {
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
				}

				hasInvalidCondition := false
				for _, cond := range plan.Status.Conditions.List {
					if cond.Type == NotValid {
						hasInvalidCondition = true
						break
					}
				}

				if shouldBeValid {
					gomega.Expect(hasInvalidCondition).To(gomega.BeFalse())
				} else {
					gomega.Expect(hasInvalidCondition).To(gomega.BeTrue())
				}
			},
			ginkgo.Entry("empty template is valid", "", true),
			ginkgo.Entry("simple valid template", "{{.VmName}}-disk-{{.DiskIndex}}", true),
			ginkgo.Entry("complex valid template", "{{.PlanName}}-{{.VmName}}-disk-{{.DiskIndex}}", true),
			ginkgo.Entry("valid template with root disk index", "{{if eq .DiskIndex .RootDiskIndex}}root{{else}}data{{end}}-{{.DiskIndex}}", true),
			ginkgo.Entry("template with invalid k8s label chars", "disk@{{.DiskIndex}}", false),
			ginkgo.Entry("template with undefined variable", "{{.UndefinedVar}}", false),
			ginkgo.Entry("template resulting in empty string", "{{if false}}disk{{end}}", false),
			ginkgo.Entry("template with special characters", "disk!{{.DiskIndex}}", false),
			ginkgo.Entry("template with spaces", "disk {{.DiskIndex}}", false),
			ginkgo.Entry("template with invalid start character", "_{{.VmName}}", false),
			ginkgo.Entry("template exceeding length limit", "very-very-very-very-very-very-very-very-very-very-long-prefix-{{.VmName}}", false),
			ginkgo.Entry("template with slash character", "{{.VmName}}/{{.DiskIndex}}", false),
		)
	})

	ginkgo.Describe("IsValidPVCNameTemplate", func() {
		var reconciler *Reconciler

		ginkgo.BeforeEach(func() {
			reconciler = &Reconciler{
				Reconciler: base.Reconciler{
					Log: planValidationLog,
				},
			}
		})

		ginkgo.DescribeTable("should validate PVC name template correctly",
			func(template string, shouldBeValid bool) {
				err := reconciler.IsValidPVCNameTemplate(template)
				if shouldBeValid {
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
				} else {
					gomega.Expect(err).To(gomega.HaveOccurred())
				}
			},
			ginkgo.Entry("empty template is valid", "", true),
			ginkgo.Entry("simple valid template", "{{.VmName}}-disk-{{.DiskIndex}}", true),
			ginkgo.Entry("complex valid template", "{{.PlanName}}-{{.VmName}}-disk-{{.DiskIndex}}", true),
			ginkgo.Entry("valid template with root disk index", "{{if eq .DiskIndex .RootDiskIndex}}root{{else}}data{{end}}-{{.DiskIndex}}", true),
			ginkgo.Entry("invalid template syntax", "{{.VmName}-disk-{{.DiskIndex}", false),
			ginkgo.Entry("template with invalid k8s label chars", "disk@{{.DiskIndex}}", false),
			ginkgo.Entry("template with undefined variable", "{{.UndefinedVar}}", false),
			ginkgo.Entry("template resulting in empty string", "{{if false}}disk{{end}}", false),
			ginkgo.Entry("template starting with non-alphanumeric", "-{{.VmName}}", false),
			ginkgo.Entry("template ending with non-alphanumeric", "{{.VmName}}-", false),
			ginkgo.Entry("template with too long result", "very-long-prefix-that-will-definitely-exceed-kubernetes-label-length-limit-for-sure-{{.VmName}}", false),
			ginkgo.Entry("template with invalid character in the middle", "disk-{{.VmName}}/{{.DiskIndex}}", false),
			ginkgo.Entry("template with uppercase characters (invalid K8s name)", "DISK-{{.VmName}}", false),
		)
	})
})

//nolint:errcheck
func createFakeReconciler(objects ...runtime.Object) *Reconciler {
	objs := []runtime.Object{}
	objs = append(objs, objects...)

	scheme := runtime.NewScheme()
	_ = core.AddToScheme(scheme)
	api.SchemeBuilder.AddToScheme(scheme)

	client := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	return &Reconciler{
		base.Reconciler{
			Client: client,
			Log:    planValidationLog,
		},
	}
}

func createProvider(name, namespace, url string, providerType api.ProviderType, secret *core.ObjectReference) *api.Provider {
	return &api.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: api.ProviderSpec{
			Type:   ptr.To(providerType),
			URL:    url,
			Secret: *secret,
		},
	}
}

func createSecret(name, namespace string, insecure bool) *core.Secret {
	return &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			insecureSkipKey: []byte(strconv.FormatBool(insecure)),
			tokenKey:        []byte(tokenValue),
		},
	}
}

func createPlan(name, namespace string, source, destination *api.Provider) *api.Plan {
	return &api.Plan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: api.PlanSpec{
			Provider: provider.Pair{
				Source: core.ObjectReference{
					Name:      source.Name,
					Namespace: source.Namespace,
				},
				Destination: core.ObjectReference{
					Name:      destination.Name,
					Namespace: destination.Namespace,
				},
			},
		},
		Referenced: api.Referenced{
			Provider: struct {
				Source      *api.Provider
				Destination *api.Provider
			}{
				Source:      source,
				Destination: destination,
			},
		},
	}
}

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
