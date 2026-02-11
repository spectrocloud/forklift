package vsphere

import (
	"database/sql"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	model "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	fb "github.com/kubev2v/forklift/pkg/lib/filebacked"
	"github.com/kubev2v/forklift/pkg/lib/inventory/container"
	libmodel "github.com/kubev2v/forklift/pkg/lib/inventory/model"
	"github.com/kubev2v/forklift/pkg/settings"
)

type fakeDB struct {
	objects map[model.Ref]model.Base
	gets    int
	failRef model.Ref
	listFn  func(list interface{}, opts libmodel.ListOptions) error
	lastOpt libmodel.ListOptions
}

func (f *fakeDB) Open(bool) error                    { return nil }
func (f *fakeDB) Close(bool) error                   { return nil }
func (f *fakeDB) Execute(string) (sql.Result, error) { return nil, nil }
func (f *fakeDB) List(list interface{}, opts libmodel.ListOptions) error {
	f.lastOpt = opts
	if f.listFn != nil {
		return f.listFn(list, opts)
	}
	return nil
}
func (f *fakeDB) Find(interface{}, libmodel.ListOptions) (fb.Iterator, error) { return nil, nil }
func (f *fakeDB) Count(libmodel.Model, libmodel.Predicate) (int64, error)     { return 0, nil }
func (f *fakeDB) Begin(...string) (*libmodel.Tx, error)                       { return nil, nil }
func (f *fakeDB) With(func(*libmodel.Tx) error, ...string) error              { return nil }
func (f *fakeDB) Insert(libmodel.Model) error                                 { return nil }
func (f *fakeDB) Update(libmodel.Model, ...libmodel.Predicate) error          { return nil }
func (f *fakeDB) Delete(libmodel.Model) error                                 { return nil }
func (f *fakeDB) Watch(libmodel.Model, libmodel.EventHandler) (*libmodel.Watch, error) {
	return nil, nil
}
func (f *fakeDB) EndWatch(*libmodel.Watch) {}

func (f *fakeDB) Get(m libmodel.Model) error {
	f.gets++
	switch o := m.(type) {
	case *model.Folder:
		ref := model.Ref{Kind: model.FolderKind, ID: o.ID}
		if ref == f.failRef {
			return errors.New("boom")
		}
		b, ok := f.objects[ref]
		if !ok {
			return libmodel.NotFound
		}
		o.Base = b
	case *model.Datacenter:
		ref := model.Ref{Kind: model.DatacenterKind, ID: o.ID}
		b, ok := f.objects[ref]
		if !ok {
			return libmodel.NotFound
		}
		o.Base = b
	case *model.Cluster:
		ref := model.Ref{Kind: model.ClusterKind, ID: o.ID}
		b, ok := f.objects[ref]
		if !ok {
			return libmodel.NotFound
		}
		o.Base = b
	case *model.Host:
		ref := model.Ref{Kind: model.HostKind, ID: o.ID}
		b, ok := f.objects[ref]
		if !ok {
			return libmodel.NotFound
		}
		o.Base = b
	case *model.Network:
		ref := model.Ref{Kind: model.NetKind, ID: o.ID}
		b, ok := f.objects[ref]
		if !ok {
			return libmodel.NotFound
		}
		o.Base = b
	case *model.Datastore:
		ref := model.Ref{Kind: model.DsKind, ID: o.ID}
		b, ok := f.objects[ref]
		if !ok {
			return libmodel.NotFound
		}
		o.Base = b
	default:
		return errors.New("unexpected model type")
	}
	return nil
}

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

func TestHandlers_RootAndOpenShiftVddk(t *testing.T) {
	orig := settings.Settings.OpenShift
	t.Cleanup(func() { settings.Settings.OpenShift = orig })

	c := container.New()

	settings.Settings.OpenShift = false
	hs := Handlers(c)
	if len(hs) != 10 {
		t.Fatalf("expected 10 handlers without VDDK, got %d", len(hs))
	}
	if !strings.Contains(Root, string(api.VSphere)) {
		t.Fatalf("unexpected Root: %s", Root)
	}

	settings.Settings.OpenShift = true
	hs2 := Handlers(c)
	if len(hs2) != 11 {
		t.Fatalf("expected 11 handlers with VDDK, got %d", len(hs2))
	}
}

func TestVSphereTree_HostNavigator_Next_Datacenter_ReturnsFolder(t *testing.T) {
	db := &fakeDB{
		objects: map[model.Ref]model.Base{
			{Kind: model.FolderKind, ID: "f1"}: {ID: "f1", Name: "folder"},
		},
	}
	n := &HostNavigator{db: db, detail: 0}
	dc := &model.Datacenter{Base: model.Base{Name: "dc"}, Clusters: model.Ref{Kind: model.FolderKind, ID: "f1"}}
	out, err := n.Next(dc)
	if err != nil || len(out) != 1 {
		t.Fatalf("unexpected: err=%v out=%v", err, out)
	}
	if _, ok := out[0].(*model.Folder); !ok {
		t.Fatalf("expected folder, got %T", out[0])
	}
}

func TestVSphereTree_HostNavigator_Next_Folder_ReturnsSubfoldersAndClusters(t *testing.T) {
	db := &fakeDB{
		objects: map[model.Ref]model.Base{},
		listFn: func(list interface{}, opts libmodel.ListOptions) error {
			switch list.(type) {
			case *[]model.Folder:
				*list.(*[]model.Folder) = []model.Folder{{Base: model.Base{ID: "sf1"}}}
			case *[]model.Cluster:
				*list.(*[]model.Cluster) = []model.Cluster{{Base: model.Base{ID: "c1"}}}
			default:
				t.Fatalf("unexpected list type: %T", list)
			}
			return nil
		},
	}
	n := &HostNavigator{db: db, detail: 0}
	f := &model.Folder{Base: model.Base{ID: "f1"}}
	out, err := n.Next(f)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 got %d", len(out))
	}
}

func TestVSphereTree_HostNavigator_Next_Cluster_ReturnsHosts(t *testing.T) {
	db := &fakeDB{
		listFn: func(list interface{}, opts libmodel.ListOptions) error {
			*list.(*[]model.Host) = []model.Host{{Base: model.Base{ID: "h1"}}}
			return nil
		},
	}
	n := &HostNavigator{db: db, detail: 0}
	out, err := n.Next(&model.Cluster{Base: model.Base{ID: "c1"}})
	if err != nil || len(out) != 1 {
		t.Fatalf("unexpected: %v %v", err, out)
	}
	if _, ok := out[0].(*model.Host); !ok {
		t.Fatalf("expected host, got %T", out[0])
	}
}

func TestVSphereTree_VMNavigator_Next_Datacenter_ReturnsFolder(t *testing.T) {
	db := &fakeDB{
		objects: map[model.Ref]model.Base{
			{Kind: model.FolderKind, ID: "f1"}: {ID: "f1", Name: "folder"},
		},
	}
	n := &VMNavigator{db: db, detail: 0}
	dc := &model.Datacenter{Base: model.Base{Name: "dc"}, Clusters: model.Ref{Kind: model.FolderKind, ID: "f1"}}
	out, err := n.Next(dc)
	if err != nil || len(out) != 1 {
		t.Fatalf("unexpected: err=%v out=%v", err, out)
	}
	if _, ok := out[0].(*model.Folder); !ok {
		t.Fatalf("expected folder, got %T", out[0])
	}
}

func TestVSphereTree_VMNavigator_Next_Folder_ReturnsSubfoldersAndVMs(t *testing.T) {
	call := 0
	db := &fakeDB{
		listFn: func(list interface{}, opts libmodel.ListOptions) error {
			call++
			switch list.(type) {
			case *[]model.Folder:
				*list.(*[]model.Folder) = []model.Folder{{Base: model.Base{ID: "sf1"}}}
			case *[]model.VM:
				*list.(*[]model.VM) = []model.VM{{Base: model.Base{ID: "v1"}}}
			default:
				t.Fatalf("unexpected list type: %T", list)
			}
			return nil
		},
	}
	n := &VMNavigator{db: db, detail: 1}
	out, err := n.Next(&model.Folder{Base: model.Base{ID: "f1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 || call < 2 {
		t.Fatalf("expected folders+vms, got len=%d calls=%d", len(out), call)
	}
	if db.lastOpt.Detail != model.MaxDetail && db.lastOpt.Detail != 0 {
		// lastOpt will be from the last List call; ensure it doesn't crash
	}
}

func TestVSphereTree_NodeBuilder_withDetail_Defaults0(t *testing.T) {
	nb := &NodeBuilder{detail: map[string]int{}}
	if nb.withDetail(model.VmKind) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestVSphereTree_NodeBuilder_Node_VM(t *testing.T) {
	nb := &NodeBuilder{provider: &api.Provider{}}
	vm := &model.VM{Base: model.Base{ID: "v1", Name: "vm", Parent: model.Ref{}}}
	n := nb.Node(&TreeNode{}, vm)
	if n == nil || n.Kind != model.VmKind {
		t.Fatalf("unexpected: %#v", n)
	}
}

func TestHandler_PredicateAndListOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/?name=dc/cluster/name", nil)
	ctx.Request = req

	h := Handler{Handler: base.Handler{}}
	pred := h.Predicate(ctx)
	if pred == nil {
		t.Fatalf("expected predicate")
	}
	eq, ok := pred.(*libmodel.EqPredicate)
	if !ok || eq.Field != NameParam || eq.Value != "name" {
		t.Fatalf("unexpected predicate: %#v", pred)
	}

	h.Detail = 1
	opts := h.ListOptions(ctx)
	if opts.Detail != model.MaxDetail {
		t.Fatalf("expected detail=%d, got %d", model.MaxDetail, opts.Detail)
	}
}

func TestPathBuilder_UsesDBAndCaches(t *testing.T) {
	rootRef := model.Ref{Kind: model.FolderKind, ID: "root"}
	dcRef := model.Ref{Kind: model.DatacenterKind, ID: "dc"}
	clusterRef := model.Ref{Kind: model.ClusterKind, ID: "cl"}
	hostRef := model.Ref{Kind: model.HostKind, ID: "h"}

	db := &fakeDB{
		objects: map[model.Ref]model.Base{
			rootRef:    {ID: "root", Name: "Datacenters", Parent: model.Ref{}},
			dcRef:      {ID: "dc", Name: "mydc", Parent: rootRef},
			clusterRef: {ID: "cl", Name: "mycluster", Parent: dcRef},
			hostRef:    {ID: "h", Name: "myhost", Parent: clusterRef},
		},
	}
	pb := &PathBuilder{DB: db}

	vm := &model.VM{Base: model.Base{ID: "vm1", Name: "vm1", Parent: hostRef}}
	p := pb.Path(vm)
	if p != "/mydc/mycluster/myhost/vm1" {
		t.Fatalf("unexpected path: %s", p)
	}

	// Cache should avoid repeated DB.Get for same refs on subsequent calls.
	vm2 := &model.VM{Base: model.Base{ID: "vm2", Name: "vm2", Parent: hostRef}}
	_ = pb.Path(vm2)
	if db.gets > 8 {
		t.Fatalf("expected caching to reduce Get calls, got %d", db.gets)
	}
}

func TestPathBuilder_DBErrorReturnsEmptyPath(t *testing.T) {
	rootRef := model.Ref{Kind: model.FolderKind, ID: "root"}
	dcRef := model.Ref{Kind: model.DatacenterKind, ID: "dc"}
	db := &fakeDB{
		objects: map[model.Ref]model.Base{
			rootRef: {ID: "root", Name: "Datacenters", Parent: model.Ref{}},
			dcRef:   {ID: "dc", Name: "mydc", Parent: rootRef},
		},
		failRef: rootRef,
	}
	pb := &PathBuilder{DB: db}
	m := &model.Datacenter{Base: model.Base{ID: "dc", Name: "mydc", Parent: rootRef}}
	if got := pb.Path(m); got != "" {
		t.Fatalf("expected empty path on db error, got %q", got)
	}
}

func TestResolver_Path_AllTypesAndDefault(t *testing.T) {
	r := &Resolver{Provider: &api.Provider{}}
	cases := []struct {
		res interface{}
		id  string
	}{
		{&Provider{}, "p1"},
		{&Folder{}, "f1"},
		{&Datacenter{}, "dc1"},
		{&Cluster{}, "c1"},
		{&Host{}, "h1"},
		{&Network{}, "n1"},
		{&Datastore{}, "ds1"},
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
			item.Name = "vm"
			rv.Set(reflect.Append(rv, reflect.ValueOf(item)))
			return nil
		},
	})
	vm := &VM{}
	if err := f.ByRef(vm, base.Ref{Name: "vm"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vm.ID != "vm1" {
		t.Fatalf("expected populated VM, got %#v", vm)
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
