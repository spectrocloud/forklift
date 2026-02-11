package ocp

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	model "github.com/kubev2v/forklift/pkg/controller/provider/model/ocp"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	cnv "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeClient struct {
	getFn   func(resource interface{}, id string) error
	listFn  func(list interface{}, param ...base.Param) error
	lastReq []base.Param
}

func (f *fakeClient) Finder() base.Finder { return &Finder{} }
func (f *fakeClient) Get(resource interface{}, id string) error {
	return f.getFn(resource, id)
}
func (f *fakeClient) List(list interface{}, param ...base.Param) error {
	f.lastReq = append([]base.Param{}, param...)
	return f.listFn(list, param...)
}
func (f *fakeClient) Watch(resource interface{}, h base.EventHandler) (*base.Watch, error) {
	return nil, nil
}
func (f *fakeClient) Find(resource interface{}, ref base.Ref) error {
	return liberr.New("not implemented")
}
func (f *fakeClient) VM(ref *base.Ref) (interface{}, error) {
	return nil, liberr.New("not implemented")
}
func (f *fakeClient) Workload(ref *base.Ref) (interface{}, error) {
	return nil, liberr.New("not implemented")
}
func (f *fakeClient) Network(ref *base.Ref) (interface{}, error) {
	return nil, liberr.New("not implemented")
}
func (f *fakeClient) Storage(ref *base.Ref) (interface{}, error) {
	return nil, liberr.New("not implemented")
}
func (f *fakeClient) Host(ref *base.Ref) (interface{}, error) {
	return nil, liberr.New("not implemented")
}

func TestResolver_Path_AllTypesAndDefault(t *testing.T) {
	r := &Resolver{Provider: &api.Provider{}}
	cases := []struct {
		res interface{}
		id  string
	}{
		{&Provider{}, "p1"},
		{&Namespace{}, "ns1"},
		{&StorageClass{}, "sc1"},
		{&NetworkAttachmentDefinition{}, "nad1"},
		{&InstanceType{}, "it1"},
		{&ClusterInstanceType{}, "cit1"},
		{&VM{}, "vm1"},
	}
	for _, tc := range cases {
		path, err := r.Path(tc.res, tc.id)
		if err != nil || path == "" || strings.HasSuffix(path, "/") {
			t.Fatalf("unexpected: path=%q err=%v", path, err)
		}
	}
	_, err := r.Path(struct{}{}, "x")
	if err == nil {
		t.Fatalf("expected resource not resolved error")
	}
	var rn base.ResourceNotResolvedError
	if !errors.As(err, &rn) {
		t.Fatalf("expected ResourceNotResolvedError, got %T %v", err, err)
	}
}

func TestFinder_ByRef_NAD_SplitsNamespaceAndName(t *testing.T) {
	f := &Finder{}

	fc := &fakeClient{
		getFn: func(resource interface{}, id string) error { return nil },
		listFn: func(list interface{}, param ...base.Param) error {
			// Provide a single match so finder populates the resource.
			rv := reflect.ValueOf(list).Elem()
			item := NetworkAttachmentDefinition{}
			item.UID = "nad1"
			rv.Set(reflect.Append(rv, reflect.ValueOf(item)))
			return nil
		},
	}
	f.With(fc)

	nad := &NetworkAttachmentDefinition{}
	if err := f.ByRef(nad, base.Ref{Name: "ns1/nad-name"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nad.UID != "nad1" {
		t.Fatalf("expected populated NAD, got %#v", nad)
	}

	// Ensure NsParam and NameParam were passed.
	var gotNs, gotName bool
	for _, p := range fc.lastReq {
		if p.Key == NsParam && p.Value == "ns1" {
			gotNs = true
		}
		if p.Key == NameParam && p.Value == "nad-name" {
			gotName = true
		}
	}
	if !gotNs || !gotName {
		t.Fatalf("expected NsParam+NameParam, got %#v", fc.lastReq)
	}
}

func ocpTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = cnv.AddToScheme(s)
	return s
}

func TestOCPTree_BranchNavigator_Next_Namespace_ReturnsVMModels(t *testing.T) {
	s := ocpTestScheme(t)
	vm1 := &cnv.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: "ns1"}}
	vm2 := &cnv.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm2", Namespace: "ns1"}}
	vmOther := &cnv.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm3", Namespace: "ns2"}}
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(vm1, vm2, vmOther).Build()

	n := &BranchNavigator{client: cl, detail: 0}
	out, err := n.Next(&model.Namespace{Base: model.Base{Name: "ns1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 got %d", len(out))
	}
	for i := range out {
		vm, ok := out[i].(*model.VM)
		if !ok {
			t.Fatalf("expected *model.VM got %T", out[i])
		}
		if vm.Namespace != "ns1" {
			t.Fatalf("unexpected vm namespace: %#v", vm)
		}
	}
}

func TestOCPTree_BranchNavigator_Next_NonNamespace_ReturnsNil(t *testing.T) {
	n := &BranchNavigator{client: fake.NewClientBuilder().WithScheme(ocpTestScheme(t)).Build(), detail: 0}
	out, err := n.Next(&model.VM{Base: model.Base{UID: "x"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil")
	}
}

type errListClient struct {
	client.Client
	err error
}

func (e errListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return e.err
}

func TestOCPTree_BranchNavigator_Next_ListErrorPropagates(t *testing.T) {
	n := &BranchNavigator{client: errListClient{Client: fake.NewClientBuilder().WithScheme(ocpTestScheme(t)).Build(), err: errors.New("boom")}, detail: 0}
	_, err := n.Next(&model.Namespace{Base: model.Base{Name: "ns1"}})
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestOCPTree_NodeBuilder_withDetail_Default0(t *testing.T) {
	nb := &NodeBuilder{detail: map[string]int{}}
	if nb.withDetail(model.VmKind) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestOCPTree_NodeBuilder_Node_Namespace(t *testing.T) {
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}}
	n := nb.Node(&TreeNode{}, &model.Namespace{Base: model.Base{Name: "ns1"}})
	if n == nil || n.Kind != model.NamespaceKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOCPTree_NodeBuilder_Node_VM_UsesCachedPath(t *testing.T) {
	pb := PathBuilder{cache: map[string]string{"ns1": "ns1"}}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}, pathBuilder: pb, detail: map[string]int{model.VmKind: 1}}
	n := nb.Node(&TreeNode{}, &model.VM{Base: model.Base{Namespace: "ns1", UID: "vm1", Name: "vm"}})
	if n == nil || n.Kind != model.VmKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}
