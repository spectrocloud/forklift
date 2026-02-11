package ovirt

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	model "github.com/kubev2v/forklift/pkg/controller/provider/model/ovirt"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	fb "github.com/kubev2v/forklift/pkg/lib/filebacked"
	libmodel "github.com/kubev2v/forklift/pkg/lib/inventory/model"
)

type fakeClient struct {
	getFn  func(resource interface{}, id string) error
	listFn func(list interface{}, param ...base.Param) error
}

func (f *fakeClient) Finder() base.Finder                       { return &Finder{} }
func (f *fakeClient) Get(resource interface{}, id string) error { return f.getFn(resource, id) }
func (f *fakeClient) List(list interface{}, param ...base.Param) error {
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
		{&DataCenter{}, "dc1"},
		{&Cluster{}, "c1"},
		{&Host{}, "h1"},
		{&Network{}, "n1"},
		{&StorageDomain{}, "sd1"},
		{&ServerCpu{}, "cpu1"},
		{&VM{}, "vm1"},
		{&Workload{}, "w1"},
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

func TestFinder_ByRef_VM_NameFound_NotFound_NotUnique(t *testing.T) {
	f := &Finder{}

	// ID path => Get used.
	gotGet := false
	f.With(&fakeClient{
		getFn: func(resource interface{}, id string) error {
			gotGet = true
			return nil
		},
		listFn: func(list interface{}, param ...base.Param) error { return nil },
	})
	if err := f.ByRef(&VM{}, base.Ref{ID: "id1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotGet {
		t.Fatalf("expected Get to be called")
	}

	// Name path => List used and single match populates resource.
	f.With(&fakeClient{
		getFn: func(resource interface{}, id string) error { return nil },
		listFn: func(list interface{}, param ...base.Param) error {
			rv := reflect.ValueOf(list).Elem()
			item := VM{}
			item.ID = "vm1"
			rv.Set(reflect.Append(rv, reflect.ValueOf(item)))
			return nil
		},
	})
	vm := &VM{}
	if err := f.ByRef(vm, base.Ref{Name: "vm"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vm.ID != "vm1" {
		t.Fatalf("expected resource populated, got %#v", vm)
	}

	// NotFound => 0 items.
	f.With(&fakeClient{
		getFn: func(resource interface{}, id string) error { return nil },
		listFn: func(list interface{}, param ...base.Param) error {
			reflect.ValueOf(list).Elem().Set(reflect.MakeSlice(reflect.ValueOf(list).Elem().Type(), 0, 0))
			return nil
		},
	})
	if err := f.ByRef(&VM{}, base.Ref{Name: "missing"}); err == nil {
		t.Fatalf("expected not found error")
	}

	// NotUnique => >1 items.
	f.With(&fakeClient{
		getFn: func(resource interface{}, id string) error { return nil },
		listFn: func(list interface{}, param ...base.Param) error {
			rv := reflect.ValueOf(list).Elem()
			a := VM{}
			a.ID = "a"
			b := VM{}
			b.ID = "b"
			rv.Set(reflect.Append(rv, reflect.ValueOf(a)))
			rv.Set(reflect.Append(rv, reflect.ValueOf(b)))
			return nil
		},
	})
	if err := f.ByRef(&VM{}, base.Ref{Name: "dup"}); err == nil {
		t.Fatalf("expected ref not unique error")
	}
}

type fakeTreeDB struct {
	listFn  func(list interface{}, opts libmodel.ListOptions) error
	lastOpt libmodel.ListOptions
}

func (f *fakeTreeDB) Open(bool) error                    { return nil }
func (f *fakeTreeDB) Close(bool) error                   { return nil }
func (f *fakeTreeDB) Execute(string) (sql.Result, error) { return nil, nil }
func (f *fakeTreeDB) Get(libmodel.Model) error           { return nil }
func (f *fakeTreeDB) List(list interface{}, opts libmodel.ListOptions) error {
	f.lastOpt = opts
	if f.listFn != nil {
		return f.listFn(list, opts)
	}
	return nil
}
func (f *fakeTreeDB) Find(interface{}, libmodel.ListOptions) (fb.Iterator, error) { return nil, nil }
func (f *fakeTreeDB) Count(libmodel.Model, libmodel.Predicate) (int64, error)     { return 0, nil }
func (f *fakeTreeDB) Begin(...string) (*libmodel.Tx, error)                       { return nil, nil }
func (f *fakeTreeDB) With(func(*libmodel.Tx) error, ...string) error              { return nil }
func (f *fakeTreeDB) Insert(libmodel.Model) error                                 { return nil }
func (f *fakeTreeDB) Update(libmodel.Model, ...libmodel.Predicate) error          { return nil }
func (f *fakeTreeDB) Delete(libmodel.Model) error                                 { return nil }
func (f *fakeTreeDB) Watch(libmodel.Model, libmodel.EventHandler) (*libmodel.Watch, error) {
	return nil, nil
}
func (f *fakeTreeDB) EndWatch(*libmodel.Watch) {}

func TestOvirtTree_BranchNavigator_listVM_Detail0(t *testing.T) {
	db := &fakeTreeDB{listFn: func(list interface{}, opts libmodel.ListOptions) error { return nil }}
	n := &BranchNavigator{db: db, detail: 0}
	_, _ = n.listVM(&model.Cluster{Base: model.Base{ID: "cl1"}})
	if db.lastOpt.Detail != 0 {
		t.Fatalf("expected detail=0 got %d", db.lastOpt.Detail)
	}
	eq, ok := db.lastOpt.Predicate.(*libmodel.EqPredicate)
	if !ok || eq.Field != "Cluster" || eq.Value != "cl1" {
		t.Fatalf("unexpected predicate: %#v", db.lastOpt.Predicate)
	}
}

func TestOvirtTree_BranchNavigator_listVM_DetailMaxWhenDetailPositive(t *testing.T) {
	db := &fakeTreeDB{listFn: func(list interface{}, opts libmodel.ListOptions) error { return nil }}
	n := &BranchNavigator{db: db, detail: 1}
	_, _ = n.listVM(&model.Cluster{Base: model.Base{ID: "cl1"}})
	if db.lastOpt.Detail != model.MaxDetail {
		t.Fatalf("expected detail=%d got %d", model.MaxDetail, db.lastOpt.Detail)
	}
}

func TestOvirtTree_BranchNavigator_Next_DataCenter_ReturnsClusters(t *testing.T) {
	db := &fakeTreeDB{
		listFn: func(list interface{}, opts libmodel.ListOptions) error {
			ptr := list.(*[]model.Cluster)
			*ptr = []model.Cluster{{Base: model.Base{ID: "c1"}}, {Base: model.Base{ID: "c2"}}}
			return nil
		},
	}
	n := &BranchNavigator{db: db, detail: 0}
	out, err := n.Next(&model.DataCenter{Base: model.Base{ID: "dc1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 got %d", len(out))
	}
	if _, ok := out[0].(*model.Cluster); !ok {
		t.Fatalf("expected *Cluster got %T", out[0])
	}
}

func TestOvirtTree_BranchNavigator_Next_Cluster_ReturnsHostsAndVMs(t *testing.T) {
	call := 0
	db := &fakeTreeDB{
		listFn: func(list interface{}, opts libmodel.ListOptions) error {
			call++
			switch list.(type) {
			case *[]model.Host:
				ptr := list.(*[]model.Host)
				*ptr = []model.Host{{Base: model.Base{ID: "h1"}}}
			case *[]model.VM:
				ptr := list.(*[]model.VM)
				*ptr = []model.VM{{Base: model.Base{ID: "v1"}}}
			default:
				t.Fatalf("unexpected list type: %T", list)
			}
			return nil
		},
	}
	n := &BranchNavigator{db: db, detail: 0}
	out, err := n.Next(&model.Cluster{Base: model.Base{ID: "cl1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 got %d", len(out))
	}
	if call < 2 {
		t.Fatalf("expected both listHost and listVM calls")
	}
}

func TestOvirtTree_BranchNavigator_Next_Cluster_HostListErrorStops(t *testing.T) {
	db := &fakeTreeDB{
		listFn: func(list interface{}, opts libmodel.ListOptions) error {
			if _, ok := list.(*[]model.Host); ok {
				return errors.New("boom")
			}
			t.Fatalf("should not list VMs after host error")
			return nil
		},
	}
	n := &BranchNavigator{db: db, detail: 0}
	_, err := n.Next(&model.Cluster{Base: model.Base{ID: "cl1"}})
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestOvirtTree_NodeBuilder_withDetail_ReturnsMapped(t *testing.T) {
	nb := &NodeBuilder{detail: map[string]int{model.VmKind: 3}}
	if nb.withDetail(model.VmKind) != 3 {
		t.Fatalf("expected 3")
	}
}

func TestOvirtTree_NodeBuilder_withDetail_Returns0WhenMissing(t *testing.T) {
	nb := &NodeBuilder{detail: map[string]int{}}
	if nb.withDetail(model.VmKind) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestOvirtTree_NodeBuilder_Node_DataCenter(t *testing.T) {
	pb := PathBuilder{cache: map[string]string{"dc1": "dc"}}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}, pathBuilder: pb}
	n := nb.Node(&TreeNode{}, &model.DataCenter{Base: model.Base{ID: "dc1", Name: "dc"}})
	if n == nil || n.Kind != model.DataCenterKind {
		t.Fatalf("unexpected: %#v", n)
	}
}

func TestOvirtTree_NodeBuilder_Node_Cluster(t *testing.T) {
	pb := PathBuilder{cache: map[string]string{"dc1": "dc"}}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}, pathBuilder: pb}
	n := nb.Node(&TreeNode{}, &model.Cluster{Base: model.Base{ID: "cl1", Name: "cl"}, DataCenter: "dc1"})
	if n == nil || n.Kind != model.ClusterKind {
		t.Fatalf("unexpected: %#v", n)
	}
}

func TestOvirtTree_NodeBuilder_Node_VM(t *testing.T) {
	pb := PathBuilder{cache: map[string]string{"cl1": "cl"}}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}, pathBuilder: pb}
	n := nb.Node(&TreeNode{}, &model.VM{Base: model.Base{ID: "vm1", Name: "vm"}, Cluster: "cl1"})
	if n == nil || n.Kind != model.VmKind {
		t.Fatalf("unexpected: %#v", n)
	}
}

func TestOvirtTree_NodeBuilder_Node_Host(t *testing.T) {
	pb := PathBuilder{cache: map[string]string{"cl1": "cl"}}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}, pathBuilder: pb}
	n := nb.Node(&TreeNode{}, &model.Host{Base: model.Base{ID: "h1", Name: "h"}, Cluster: "cl1"})
	if n == nil || n.Kind != model.HostKind {
		t.Fatalf("unexpected: %#v", n)
	}
}

func TestOvirtTree_NodeBuilder_Node_Network(t *testing.T) {
	pb := PathBuilder{cache: map[string]string{"dc1": "dc"}}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}, pathBuilder: pb}
	n := nb.Node(&TreeNode{}, &model.Network{Base: model.Base{ID: "n1", Name: "n"}, DataCenter: "dc1"})
	if n == nil || n.Kind != model.NetKind {
		t.Fatalf("unexpected: %#v", n)
	}
}

func TestOvirtTree_NodeBuilder_Node_StorageDomain(t *testing.T) {
	pb := PathBuilder{cache: map[string]string{"dc1": "dc"}}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}, pathBuilder: pb}
	n := nb.Node(&TreeNode{}, &model.StorageDomain{Base: model.Base{ID: "sd1", Name: "sd"}, DataCenter: "dc1"})
	if n == nil || n.Kind != model.StorageKind {
		t.Fatalf("unexpected: %#v", n)
	}
}

func TestOvirtTree_NodeBuilder_Node_ServerCpu(t *testing.T) {
	pb := PathBuilder{cache: map[string]string{"dc1": "dc"}}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}, pathBuilder: pb}
	n := nb.Node(&TreeNode{}, &model.ServerCpu{Base: model.Base{ID: "cpu1", Name: "cpu"}, DataCenter: "dc1"})
	if n == nil || n.Kind != model.ServerCPUKind {
		t.Fatalf("unexpected: %#v", n)
	}
}
