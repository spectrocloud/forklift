package ova

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	model "github.com/kubev2v/forklift/pkg/controller/provider/model/ova"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	"github.com/kubev2v/forklift/pkg/lib/inventory/container"
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

func TestHandlersAndRoot(t *testing.T) {
	hs := Handlers(container.New())
	if len(hs) != 7 {
		t.Fatalf("expected 7 handlers, got %d", len(hs))
	}
	if !strings.Contains(Root, string(api.Ova)) {
		t.Fatalf("unexpected Root: %s", Root)
	}
}

func TestResolver_Path_AndDefault(t *testing.T) {
	r := &Resolver{Provider: &api.Provider{}}
	cases := []struct {
		res interface{}
		id  string
	}{
		{&Provider{}, "p1"},
		{&Network{}, "n1"},
		{&VM{}, "vm1"},
		{&Disk{}, "d1"},
		{&Workload{}, "w1"},
		{&Storage{}, "s1"},
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

func TestFinder_ByRef_Network_NameFound_NotFound_NotUnique(t *testing.T) {
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
	if err := f.ByRef(&Network{}, base.Ref{ID: "id1"}); err != nil {
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
			item := Network{}
			item.ID = "n1"
			rv.Set(reflect.Append(rv, reflect.ValueOf(item)))
			return nil
		},
	})
	n := &Network{}
	if err := f.ByRef(n, base.Ref{Name: "net"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.ID != "n1" {
		t.Fatalf("expected resource populated, got %#v", n)
	}

	// NotFound => 0 items.
	f.With(&fakeClient{
		getFn: func(resource interface{}, id string) error { return nil },
		listFn: func(list interface{}, param ...base.Param) error {
			reflect.ValueOf(list).Elem().Set(reflect.MakeSlice(reflect.ValueOf(list).Elem().Type(), 0, 0))
			return nil
		},
	})
	if err := f.ByRef(&Network{}, base.Ref{Name: "missing"}); err == nil {
		t.Fatalf("expected not found error")
	}

	// NotUnique => >1 items.
	f.With(&fakeClient{
		getFn: func(resource interface{}, id string) error { return nil },
		listFn: func(list interface{}, param ...base.Param) error {
			rv := reflect.ValueOf(list).Elem()
			a := Network{}
			a.ID = "a"
			b := Network{}
			b.ID = "b"
			rv.Set(reflect.Append(rv, reflect.ValueOf(a)))
			rv.Set(reflect.Append(rv, reflect.ValueOf(b)))
			return nil
		},
	})
	if err := f.ByRef(&Network{}, base.Ref{Name: "dup"}); err == nil {
		t.Fatalf("expected ref not unique error")
	}
}

func TestOVATree_TreeHandler_List_MethodNotAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	h := TreeHandler{}
	h.List(ctx)
	ctx.Writer.WriteHeaderNow()
	if ctx.Writer.Status() != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 got %d", ctx.Writer.Status())
	}
}

func TestOVATree_TreeHandler_Get_MethodNotAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	h := TreeHandler{}
	h.Get(ctx)
	ctx.Writer.WriteHeaderNow()
	if ctx.Writer.Status() != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 got %d", ctx.Writer.Status())
	}
}

func TestOVATree_NodeBuilder_withDetail_Default0(t *testing.T) {
	nb := &NodeBuilder{detail: map[string]int{}}
	if nb.withDetail(model.VmKind) != 0 {
		t.Fatalf("expected 0")
	}
}

func TestOVATree_NodeBuilder_Node_VM(t *testing.T) {
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}}
	n := nb.Node(&TreeNode{}, &model.VM{Base: model.Base{ID: "vm1", Name: "vm"}})
	if n == nil || n.Kind != model.VmKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOVATree_NodeBuilder_Node_Network(t *testing.T) {
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}}
	n := nb.Node(&TreeNode{}, &model.Network{Base: model.Base{ID: "n1", Name: "net"}})
	if n == nil || n.Kind != model.NetKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOVATree_NodeBuilder_Node_Disk(t *testing.T) {
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}}
	n := nb.Node(&TreeNode{}, &model.Disk{Base: model.Base{ID: "d1", Name: "disk"}})
	if n == nil || n.Kind != model.DiskKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}

func TestOVATree_NodeBuilder_Node_Storage(t *testing.T) {
	nb := &NodeBuilder{handler: Handler{Handler: base.Handler{Provider: &api.Provider{}}}}
	n := nb.Node(&TreeNode{}, &model.Storage{Base: model.Base{ID: "s1", Name: "st"}})
	if n == nil || n.Kind != model.StorageKind {
		t.Fatalf("unexpected node: %#v", n)
	}
}
