package plan

import (
	"strings"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	apiref "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	cnv "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVmNameHandler(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Helper function to check if VM name is a valid DNS1123 subdomain
	validateVmName := func(name string) bool {
		return len(validation.IsDNS1123Subdomain(name)) == 0
	}

	//Test all cases in name adjustments
	originalVmName := "----------------Vm!@#$%^&*()_+-Name/.is,';[]-CorREct-<>123----------------------"
	newVmName := "vm--name.is-correct-123"
	changedName := changeVmName(originalVmName)
	g.Expect(changedName).To(gomega.Equal(newVmName))
	g.Expect(validateVmName(changedName)).To(gomega.BeTrue(), "Changed name should match DNS1123 subdomain format")

	//Test the case that the VM name is empty after all removals
	emptyVM := ".__."
	newVmNameFromId := "vm-"
	changedEmptyName := changeVmName(emptyVM)
	g.Expect(changedEmptyName).To(gomega.ContainSubstring(newVmNameFromId))
	g.Expect(validateVmName(changedEmptyName)).To(gomega.BeTrue(), "Changed name from empty should match DNS1123 subdomain format")

	//Test handling of multiple consecutive dots
	multiDotVM := "mtv.func.-.rhel.-...8.8"
	expectedMultiDotResult := "mtv.func.rhel.8.8"
	changedMultiDotName := changeVmName(multiDotVM)
	g.Expect(changedMultiDotName).To(gomega.Equal(expectedMultiDotResult))
	g.Expect(validateVmName(changedMultiDotName)).To(gomega.BeTrue(), "Changed name with multiple dots should match DNS1123 subdomain format")

	multiDotVM2 := ".....mtv.func..-...............rhel.-...8.8"
	expectedMultiDotResult2 := "mtv.func.rhel.8.8"
	changedMultiDotName2 := changeVmName(multiDotVM2)
	g.Expect(changedMultiDotName2).To(gomega.Equal(expectedMultiDotResult2))
	g.Expect(validateVmName(changedMultiDotName2)).To(gomega.BeTrue(), "Changed name with multiple leading dots should match DNS1123 subdomain format")
}

// ---- Merged from vm_name_handler_more_test.go ----

func newKubevirtFakeClientWithFieldIndexes(t *testing.T, scheme *runtime.Scheme) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&cnv.VirtualMachine{}, "metadata.name", func(obj client.Object) []string {
			return []string{obj.GetName()}
		}).
		WithIndex(&cnv.VirtualMachine{}, "metadata.namespace", func(obj client.Object) []string {
			return []string{obj.GetNamespace()}
		}).
		Build()
}

func TestChangeVmName_Lowercases(t *testing.T) {
	if got := changeVmName("MyVM"); got != "myvm" {
		t.Fatalf("expected myvm got %q", got)
	}
}

func TestChangeVmName_TrimsLeadingTrailingDotsAndDashes(t *testing.T) {
	if got := changeVmName("..--MyVM--.."); got != "myvm" {
		t.Fatalf("expected myvm got %q", got)
	}
}

func TestChangeVmName_ReplacesUnderscoreWithDash(t *testing.T) {
	if got := changeVmName("a_b"); got != "a-b" {
		t.Fatalf("expected a-b got %q", got)
	}
}

func TestChangeVmName_RemovesInvalidCharacters(t *testing.T) {
	if got := changeVmName("a$#b"); got != "ab" {
		t.Fatalf("expected ab got %q", got)
	}
}

func TestChangeVmName_SplitsOnDotsAndDropsEmptyParts(t *testing.T) {
	if got := changeVmName("a..b...c"); got != "a.b.c" {
		t.Fatalf("expected a.b.c got %q", got)
	}
}

func TestChangeVmName_InnerPartTrim(t *testing.T) {
	if got := changeVmName("a.-b-.c"); got != "a.b.c" {
		t.Fatalf("expected a.b.c got %q", got)
	}
}

func TestChangeVmName_TruncatesToMaxLength(t *testing.T) {
	in := strings.Repeat("a", NameMaxLength+10)
	got := changeVmName(in)
	if len(got) != NameMaxLength {
		t.Fatalf("expected len %d got %d", NameMaxLength, len(got))
	}
}

func TestChangeVmName_EmptyBecomesVmDashSuffix(t *testing.T) {
	got := changeVmName("...---___$$$")
	if !strings.HasPrefix(got, "vm-") {
		t.Fatalf("expected prefix vm- got %q", got)
	}
	if len(got) != len("vm-")+4 {
		t.Fatalf("expected length %d got %d", len("vm-")+4, len(got))
	}
}

func TestGenerateRandVmNameSuffix_Length4(t *testing.T) {
	got := generateRandVmNameSuffix()
	if len(got) != 4 {
		t.Fatalf("expected len 4 got %d", len(got))
	}
}

func TestGenerateRandVmNameSuffix_Charset(t *testing.T) {
	got := generateRandVmNameSuffix()
	for i := 0; i < len(got); i++ {
		c := got[i]
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') {
			t.Fatalf("unexpected char %q in %q", c, got)
		}
	}
}

func TestCheckIfVmNameExistsInNamespace_ReturnsErrorWhenKubevirtTypesNotInScheme(t *testing.T) {
	scheme := runtime.NewScheme() // kubevirt types not registered => List should error
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := &plancontext.Context{Log: logging.WithName("t")}
	ctx.Destination.Client = cl
	ctx.Migration = &api.Migration{Status: api.MigrationStatus{VMs: []*planapi.VMStatus{}}}
	kv := &KubeVirt{Context: ctx}
	_, err := kv.checkIfVmNameExistsInNamespace("name", "ns")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestCheckIfVmNameExistsInNamespace_TrueWhenNameInMigrationStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = cnv.AddToScheme(scheme)
	cl := newKubevirtFakeClientWithFieldIndexes(t, scheme)
	ctx := &plancontext.Context{Log: logging.WithName("t")}
	ctx.Destination.Client = cl
	ctx.Migration = &api.Migration{Status: api.MigrationStatus{VMs: []*planapi.VMStatus{
		{VM: planapi.VM{Ref: apiref.Ref{Name: "taken"}}},
	}}}
	kv := &KubeVirt{Context: ctx}
	exists, err := kv.checkIfVmNameExistsInNamespace("taken", "ns")
	if err != nil || !exists {
		t.Fatalf("expected true nil, got %v %v", exists, err)
	}
}

func TestCheckIfVmNameExistsInNamespace_FalseWhenNoVMsAndEmptyList(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = cnv.AddToScheme(scheme)
	cl := newKubevirtFakeClientWithFieldIndexes(t, scheme)
	ctx := &plancontext.Context{Log: logging.WithName("t")}
	ctx.Destination.Client = cl
	ctx.Migration = &api.Migration{Status: api.MigrationStatus{VMs: []*planapi.VMStatus{}}}
	kv := &KubeVirt{Context: ctx}
	exists, err := kv.checkIfVmNameExistsInNamespace("free", "ns")
	if err != nil || exists {
		t.Fatalf("expected false nil, got %v %v", exists, err)
	}
}

func TestChangeVmNameDNS1123_ReturnsErrorWhenListFails(t *testing.T) {
	scheme := runtime.NewScheme() // no kubevirt types => list fails
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := &plancontext.Context{Log: logging.WithName("t")}
	ctx.Destination.Client = cl
	ctx.Migration = &api.Migration{}
	kv := &KubeVirt{Context: ctx}
	_, err := kv.changeVmNameDNS1123("MyVM", "ns")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestChangeVmNameDNS1123_NoConflict_ReturnsNormalizedName(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = cnv.AddToScheme(scheme)
	cl := newKubevirtFakeClientWithFieldIndexes(t, scheme)
	ctx := &plancontext.Context{Log: logging.WithName("t")}
	ctx.Destination.Client = cl
	ctx.Migration = &api.Migration{Status: api.MigrationStatus{VMs: []*planapi.VMStatus{}}}
	kv := &KubeVirt{Context: ctx}
	got, err := kv.changeVmNameDNS1123("My_VM", "ns")
	if err != nil || got != "my-vm" {
		t.Fatalf("expected my-vm nil, got %q %v", got, err)
	}
}

func TestChangeVmNameDNS1123_Conflict_AppendsSuffix(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = cnv.AddToScheme(scheme)
	cl := newKubevirtFakeClientWithFieldIndexes(t, scheme)
	ctx := &plancontext.Context{Log: logging.WithName("t")}
	ctx.Destination.Client = cl
	// force conflict via migration list (avoids relying on field selectors)
	ctx.Migration = &api.Migration{Status: api.MigrationStatus{VMs: []*planapi.VMStatus{
		{VM: planapi.VM{Ref: apiref.Ref{Name: "myvm"}}},
	}}}
	kv := &KubeVirt{Context: ctx}
	got, err := kv.changeVmNameDNS1123("MyVM", "ns")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasPrefix(got, "myvm-") || len(got) != len("myvm-")+4 {
		t.Fatalf("unexpected name: %q", got)
	}
}

func TestChangeVmNameDNS1123_ConflictAtMaxLength_TruncatesThenAppendsSuffix(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = cnv.AddToScheme(scheme)
	cl := newKubevirtFakeClientWithFieldIndexes(t, scheme)
	ctx := &plancontext.Context{Log: logging.WithName("t")}
	ctx.Destination.Client = cl

	base := strings.Repeat("a", NameMaxLength) // already max length after normalization
	ctx.Migration = &api.Migration{Status: api.MigrationStatus{VMs: []*planapi.VMStatus{
		{VM: planapi.VM{Ref: apiref.Ref{Name: base}}},
	}}}
	kv := &KubeVirt{Context: ctx}
	got, err := kv.changeVmNameDNS1123(base, "ns")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != NameMaxLength {
		t.Fatalf("expected len %d got %d (%q)", NameMaxLength, len(got), got)
	}
	if !strings.HasPrefix(got, strings.Repeat("a", NameMaxLength-5)+"-") {
		t.Fatalf("unexpected truncation/prefix: %q", got)
	}
}
