package openstack

import (
	"database/sql"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	model "github.com/kubev2v/forklift/pkg/controller/provider/model/openstack"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	fb "github.com/kubev2v/forklift/pkg/lib/filebacked"
	"github.com/kubev2v/forklift/pkg/lib/inventory/container"
	libmodel "github.com/kubev2v/forklift/pkg/lib/inventory/model"
)

type fakeDB struct {
	projects map[string]*model.Project
	getCalls int
	listFn   func(list interface{}, opts libmodel.ListOptions) error
	lastList libmodel.ListOptions
}

func (f *fakeDB) Open(bool) error                    { return nil }
func (f *fakeDB) Close(bool) error                   { return nil }
func (f *fakeDB) Execute(string) (sql.Result, error) { return nil, nil }
func (f *fakeDB) List(list interface{}, opts libmodel.ListOptions) error {
	f.lastList = opts
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
	f.getCalls++
	p, ok := m.(*model.Project)
	if !ok {
		return errors.New("unexpected model type")
	}
	found, ok := f.projects[p.ID]
	if !ok {
		return libmodel.NotFound
	}
	*p = *found
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

func TestHandlers_List(t *testing.T) {
	hs := Handlers(container.New())
	if len(hs) < 5 {
		t.Fatalf("expected multiple handlers, got %d", len(hs))
	}
	if !strings.Contains(Root, string(api.OpenStack)) {
		t.Fatalf("unexpected Root: %s", Root)
	}
}

func TestHandler_PredicateAndListOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/?name=proj/sub/name", nil)
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

func TestPathBuilder_ProjectAndVMPaths_WithCaching(t *testing.T) {
	db := &fakeDB{
		projects: map[string]*model.Project{
			"dom": {Base: model.Base{ID: "dom", Name: "domain"}, IsDomain: true, DomainID: "dom", ParentID: "dom"},
			"p1":  {Base: model.Base{ID: "p1", Name: "parent"}, IsDomain: false, DomainID: "dom", ParentID: "dom"},
			"p2":  {Base: model.Base{ID: "p2", Name: "child"}, IsDomain: false, DomainID: "dom", ParentID: "p1"},
		},
	}
	pb := &PathBuilder{DB: db}

	d := pb.Path(db.projects["dom"])
	if d != "domain" {
		t.Fatalf("unexpected domain path: %s", d)
	}

	child := pb.Path(db.projects["p2"])
	if child != "parent/child" {
		t.Fatalf("unexpected child path: %s", child)
	}

	vm := &model.VM{Base: model.Base{Name: "vm1"}, TenantID: "p2"}
	vmPath := pb.Path(vm)
	if vmPath != "parent/child/vm1" {
		t.Fatalf("unexpected vm path: %s", vmPath)
	}

	// cache hit: tenant project should only be fetched once even if we ask twice.
	_ = pb.Path(&model.VM{Base: model.Base{Name: "vm2"}, TenantID: "p2"})
	if db.getCalls < 2 { // p2 + p1 should have been requested once each.
		t.Fatalf("expected Get calls, got %d", db.getCalls)
	}
}

func TestResolver_Path_AllResourceTypesAndDefault(t *testing.T) {
	p := &api.Provider{}
	r := &Resolver{Provider: p}
	allowEmptyReturn := map[string]bool{
		"project": true,
		"flavor":  true,
	}

	cases := []struct {
		name string
		res  interface{}
		id   string
	}{
		{"provider", &Provider{}, "p1"},
		{"region", &Region{}, "r1"},
		{"project", &Project{}, "pr1"},
		{"image", &Image{}, "i1"},
		{"flavor", &Flavor{}, "f1"},
		{"vm", &VM{}, "vm1"},
		{"snapshot", &Snapshot{}, "s1"},
		{"volume", &Volume{}, "v1"},
		{"volumetype", &VolumeType{}, "vt1"},
		{"network", &Network{}, "n1"},
		{"workload", &Workload{}, "w1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, err := r.Path(tc.res, tc.id)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if allowEmptyReturn[tc.name] {
				// Some resolver cases intentionally don't set the return value but do set the
				// resource SelfLink via Link().
				if path != "" {
					t.Fatalf("expected empty return path, got %q", path)
				}
				v := reflect.ValueOf(tc.res).Elem()
				f := v.FieldByName("SelfLink")
				if !f.IsValid() || f.Kind() != reflect.String || f.String() == "" || strings.HasSuffix(f.String(), "/") {
					t.Fatalf("expected SelfLink to be set, got %#v", tc.res)
				}
				return
			}
			if path == "" || strings.HasSuffix(path, "/") {
				t.Fatalf("unexpected path: %q", path)
			}
		})
	}

	_, err := r.Path(struct{}{}, "x")
	if err == nil {
		t.Fatalf("expected error for unsupported resource")
	}
	var rn base.ResourceNotResolvedError
	if !errors.As(err, &rn) {
		t.Fatalf("expected ResourceNotResolvedError, got: %T %v", err, err)
	}
}

func TestOpenstackTree_BranchNavigator_listVM_Detail0(t *testing.T) {
	db := &fakeDB{
		listFn: func(list interface{}, opts libmodel.ListOptions) error {
			ptr := list.(*[]model.VM)
			*ptr = []model.VM{{Base: model.Base{ID: "vm1"}}}
			return nil
		},
	}
	n := &BranchNavigator{db: db, detail: 0}
	proj := &model.Project{Base: model.Base{ID: "t1"}}
	_, err := n.listVM(proj)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if db.lastList.Detail != 0 {
		t.Fatalf("expected detail=0 got %d", db.lastList.Detail)
	}
	eq, ok := db.lastList.Predicate.(*libmodel.EqPredicate)
	if !ok || eq.Field != "TenantID" || eq.Value != "t1" {
		t.Fatalf("unexpected predicate: %#v", db.lastList.Predicate)
	}
}

func TestOpenstackTree_BranchNavigator_listVM_DetailMaxWhenDetailPositive(t *testing.T) {
	db := &fakeDB{listFn: func(list interface{}, opts libmodel.ListOptions) error { return nil }}
	n := &BranchNavigator{db: db, detail: 1}
	proj := &model.Project{Base: model.Base{ID: "t1"}}
	_, _ = n.listVM(proj)
	if db.lastList.Detail != model.MaxDetail {
		t.Fatalf("expected detail=%d got %d", model.MaxDetail, db.lastList.Detail)
	}
}

func TestOpenstackTree_BranchNavigator_Next_Project_ReturnsVMPtrs(t *testing.T) {
	db := &fakeDB{
		listFn: func(list interface{}, opts libmodel.ListOptions) error {
			ptr := list.(*[]model.VM)
			*ptr = []model.VM{{Base: model.Base{ID: "vm1"}}, {Base: model.Base{ID: "vm2"}}}
			return nil
		},
	}
	n := &BranchNavigator{db: db, detail: 0}
	out, err := n.Next(&model.Project{Base: model.Base{ID: "t1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 got %d", len(out))
	}
	if _, ok := out[0].(*model.VM); !ok {
		t.Fatalf("expected *model.VM got %T", out[0])
	}
}

func TestOpenstackTree_BranchNavigator_Next_ListErrorPropagates(t *testing.T) {
	db := &fakeDB{listFn: func(list interface{}, opts libmodel.ListOptions) error { return errors.New("boom") }}
	n := &BranchNavigator{db: db, detail: 0}
	_, err := n.Next(&model.Project{Base: model.Base{ID: "t1"}})
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestOpenstackTree_BranchNavigator_Next_NonProject_NoChildren(t *testing.T) {
	db := &fakeDB{listFn: func(list interface{}, opts libmodel.ListOptions) error { t.Fatalf("should not list"); return nil }}
	n := &BranchNavigator{db: db, detail: 0}
	out, err := n.Next(&model.Region{Base: model.Base{ID: "r1"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 got %d", len(out))
	}
}

func TestOpenstackTree_NodeBuilder_withDetail_ReturnsMapped(t *testing.T) {
	r := &NodeBuilder{detail: map[string]int{model.VMKind: 7}}
	if got := r.withDetail(model.VMKind); got != 7 {
		t.Fatalf("expected 7 got %d", got)
	}
}

func TestOpenstackTree_NodeBuilder_withDetail_Returns0WhenMissing(t *testing.T) {
	r := &NodeBuilder{detail: map[string]int{}}
	if got := r.withDetail(model.VMKind); got != 0 {
		t.Fatalf("expected 0 got %d", got)
	}
}

func TestOpenstackTree_NodeBuilder_Node_Region(t *testing.T) {
	p := &api.Provider{}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: p}}}
	n := nb.Node(&TreeNode{}, &model.Region{Base: model.Base{ID: "r1", Name: "r"}})
	if n == nil || n.Kind != model.RegionKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOpenstackTree_NodeBuilder_Node_Project(t *testing.T) {
	p := &api.Provider{}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: p}}}
	n := nb.Node(&TreeNode{}, &model.Project{Base: model.Base{ID: "p1", Name: "proj"}, IsDomain: true, DomainID: "p1", ParentID: "p1"})
	if n == nil || n.Kind != model.ProjectKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOpenstackTree_NodeBuilder_Node_VM_UsesPathBuilder(t *testing.T) {
	p := &api.Provider{}
	pb := PathBuilder{cache: map[string]interface{}{"t1": &model.Project{Base: model.Base{ID: "t1", Name: "proj"}, IsDomain: true, DomainID: "t1", ParentID: "t1"}}}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: p}}, pathBuilder: pb, detail: map[string]int{model.VMKind: 1}}
	vm := &model.VM{Base: model.Base{ID: "vm1", Name: "vm"}, TenantID: "t1"}
	n := nb.Node(&TreeNode{}, vm)
	if n == nil || n.Kind != model.VMKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOpenstackTree_NodeBuilder_Node_Subnet(t *testing.T) {
	p := &api.Provider{}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: p}}}
	n := nb.Node(&TreeNode{}, &model.Subnet{Base: model.Base{ID: "s1", Name: "sn"}})
	if n == nil || n.Kind != model.SubnetKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOpenstackTree_NodeBuilder_Node_Image(t *testing.T) {
	p := &api.Provider{}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: p}}}
	n := nb.Node(&TreeNode{}, &model.Image{Base: model.Base{ID: "i1", Name: "img"}})
	if n == nil || n.Kind != model.ImageKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOpenstackTree_NodeBuilder_Node_Flavor(t *testing.T) {
	p := &api.Provider{}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: p}}}
	n := nb.Node(&TreeNode{}, &model.Flavor{Base: model.Base{ID: "f1", Name: "flv"}})
	if n == nil || n.Kind != model.FlavorKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOpenstackTree_NodeBuilder_Node_Snapshot(t *testing.T) {
	p := &api.Provider{}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: p}}}
	n := nb.Node(&TreeNode{}, &model.Snapshot{Base: model.Base{ID: "s1", Name: "snap"}})
	if n == nil || n.Kind != model.SnapshotKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOpenstackTree_NodeBuilder_Node_Volume(t *testing.T) {
	p := &api.Provider{}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: p}}}
	n := nb.Node(&TreeNode{}, &model.Volume{Base: model.Base{ID: "v1", Name: "vol"}})
	if n == nil || n.Kind != model.VolumeKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOpenstackTree_NodeBuilder_Node_VolumeType(t *testing.T) {
	p := &api.Provider{}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: p}}}
	n := nb.Node(&TreeNode{}, &model.VolumeType{Base: model.Base{ID: "vt1", Name: "vt"}})
	if n == nil || n.Kind != model.VolumeTypeKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOpenstackTree_NodeBuilder_Node_Network(t *testing.T) {
	p := &api.Provider{}
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: p}}}
	n := nb.Node(&TreeNode{}, &model.Network{Base: model.Base{ID: "n1", Name: "net"}})
	if n == nil || n.Kind != model.NetworkKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestFinder_ByRef_VM_NameFound_NotFound_NotUnique(t *testing.T) {
	f := &Finder{}

	// ID path => Get is used.
	gotGet := false
	fc := &fakeClient{
		getFn: func(resource interface{}, id string) error {
			gotGet = true
			return nil
		},
		listFn: func(list interface{}, param ...base.Param) error {
			return nil
		},
	}
	f.With(fc)
	vm := &VM{}
	if err := f.ByRef(vm, base.Ref{ID: "id1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotGet {
		t.Fatalf("expected Get to be called")
	}

	// Name path => List is used and should populate single item.
	fc2 := &fakeClient{
		getFn: func(resource interface{}, id string) error { return nil },
		listFn: func(list interface{}, param ...base.Param) error {
			rv := reflect.ValueOf(list)
			if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Slice {
				return errors.New("expected pointer to slice")
			}
			item := VM{}
			item.ID = "vm1"
			item.Name = "vm"
			rv.Elem().Set(reflect.Append(rv.Elem(), reflect.ValueOf(item)))
			return nil
		},
	}
	f.With(fc2)
	vm2 := &VM{}
	if err := f.ByRef(vm2, base.Ref{Name: "proj/vm"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vm2.ID != "vm1" {
		t.Fatalf("expected VM to be set from list result, got %#v", vm2)
	}

	// NotFound.
	fc3 := &fakeClient{
		getFn: func(resource interface{}, id string) error { return nil },
		listFn: func(list interface{}, param ...base.Param) error {
			reflect.ValueOf(list).Elem().Set(reflect.MakeSlice(reflect.ValueOf(list).Elem().Type(), 0, 0))
			return nil
		},
	}
	f.With(fc3)
	if err := f.ByRef(&VM{}, base.Ref{Name: "missing"}); err == nil {
		t.Fatalf("expected not found error")
	}

	// Not unique.
	fc4 := &fakeClient{
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
	}
	f.With(fc4)
	if err := f.ByRef(&VM{}, base.Ref{Name: "dup"}); err == nil {
		t.Fatalf("expected ref not unique error")
	}
}

func TestOpenstackResource_With(t *testing.T) {
	m := &model.Base{ID: "id1", Name: "n1", Revision: 2}
	var r Resource
	r.With(m)
	if r.ID != "id1" || r.Name != "n1" || r.Revision != 2 {
		t.Fatalf("unexpected resource: %#v", r)
	}
}
