package main

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	fb "github.com/kubev2v/forklift/pkg/lib/filebacked"
	"github.com/kubev2v/forklift/pkg/lib/inventory/model"
	"github.com/kubev2v/forklift/pkg/lib/inventory/web"
	"k8s.io/apimachinery/pkg/types"
)

type fakeDB struct {
	getErr    error
	listErr   error
	listOut   []Model
	getCalled bool
	lastGetID int
}

func (f *fakeDB) Open(bool) error                    { return nil }
func (f *fakeDB) Close(bool) error                   { return nil }
func (f *fakeDB) Execute(string) (sql.Result, error) { return nil, errors.New("not implemented") }
func (f *fakeDB) Get(m model.Model) error {
	f.getCalled = true
	if f.getErr != nil {
		return f.getErr
	}
	// Populate the model based on the requested ID.
	if mm, ok := m.(*Model); ok {
		f.lastGetID = mm.ID
		if mm.ID == 404 {
			return model.NotFound
		}
		mm.Name = "ok"
		mm.Age = 1
	}
	return nil
}
func (f *fakeDB) List(dst interface{}, _ model.ListOptions) error {
	if f.listErr != nil {
		return f.listErr
	}
	if out, ok := dst.(*[]Model); ok {
		*out = append((*out)[:0], f.listOut...)
		return nil
	}
	return errors.New("unexpected dst type")
}
func (f *fakeDB) Find(interface{}, model.ListOptions) (fb.Iterator, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeDB) Count(model.Model, model.Predicate) (int64, error) {
	return 0, errors.New("not implemented")
}
func (f *fakeDB) Begin(...string) (*model.Tx, error)           { return nil, errors.New("not implemented") }
func (f *fakeDB) With(func(*model.Tx) error, ...string) error  { return errors.New("not implemented") }
func (f *fakeDB) Insert(model.Model) error                     { return errors.New("not implemented") }
func (f *fakeDB) Update(model.Model, ...model.Predicate) error { return errors.New("not implemented") }
func (f *fakeDB) Delete(model.Model) error                     { return errors.New("not implemented") }
func (f *fakeDB) Watch(model.Model, model.EventHandler) (*model.Watch, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeDB) EndWatch(*model.Watch) {}

func TestModel_PkAndString(t *testing.T) {
	m := &Model{ID: 7, Name: "bob"}
	if m.Pk() != "7" {
		t.Fatalf("unexpected pk: %s", m.Pk())
	}
	if s := m.String(); s == "" {
		t.Fatalf("expected non-empty string")
	}
}

func TestEventHandler_BasicFlow(t *testing.T) {
	h := &EventHandler{
		options: web.WatchOptions{Snapshot: true},
	}
	if h.Options().Snapshot != true {
		t.Fatalf("unexpected options: %#v", h.Options())
	}
	h.Started(9)
	if !h.started || h.wid != 9 {
		t.Fatalf("expected started")
	}
	h.Parity()
	if !h.parity {
		t.Fatalf("expected parity")
	}
	h.Created(web.Event{Resource: &Model{ID: 1}})
	h.Updated(web.Event{Resource: &Model{ID: 2}})
	h.Deleted(web.Event{Resource: &Model{ID: 3}})
	if len(h.created) != 1 || h.created[0] != 1 {
		t.Fatalf("unexpected created: %#v", h.created)
	}
	if len(h.updated) != 1 || h.updated[0] != 2 {
		t.Fatalf("unexpected updated: %#v", h.updated)
	}
	if len(h.deleted) != 1 || h.deleted[0] != 3 {
		t.Fatalf("unexpected deleted: %#v", h.deleted)
	}
	h.End()
	if !h.done {
		t.Fatalf("expected done")
	}
}

func TestEndpoint_Get(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/?id=404", nil)
		if q := c.Query("id"); q != "404" {
			t.Fatalf("expected query id=404, got %q", q)
		}

		fdb := &fakeDB{}
		e := Endpoint{db: fdb}
		e.Get(c)
		if !fdb.getCalled || fdb.lastGetID != 404 {
			t.Fatalf("expected Get called with 404, called=%v id=%d", fdb.getCalled, fdb.lastGetID)
		}
		c.Writer.WriteHeaderNow()
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 got %d", w.Code)
		}
	})

	t.Run("internal error", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/?id=1", nil)
		if q := c.Query("id"); q != "1" {
			t.Fatalf("expected query id=1, got %q", q)
		}

		fdb := &fakeDB{getErr: errors.New("boom")}
		e := Endpoint{db: fdb}
		e.Get(c)
		if !fdb.getCalled {
			t.Fatalf("expected Get called")
		}
		c.Writer.WriteHeaderNow()
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/?id=1", nil)
		if q := c.Query("id"); q != "1" {
			t.Fatalf("expected query id=1, got %q", q)
		}

		fdb := &fakeDB{}
		e := Endpoint{db: fdb}
		e.Get(c)
		if !fdb.getCalled || fdb.lastGetID != 1 {
			t.Fatalf("expected Get called with 1, called=%v id=%d", fdb.getCalled, fdb.lastGetID)
		}
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 got %d", w.Code)
		}
	})
}

func TestEndpoint_List(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("list error => 500", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/models", nil)
		e := Endpoint{db: &fakeDB{listErr: errors.New("boom")}}
		e.List(c)
		c.Writer.WriteHeaderNow()
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 got %d", w.Code)
		}
	})

	t.Run("list success => 200", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/models", nil)
		e := Endpoint{db: &fakeDB{listOut: []Model{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}}}
		e.List(c)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 got %d", w.Code)
		}
	})
}

func TestCollector_Basics(t *testing.T) {
	c := &Collector{db: &fakeDB{}}
	if c.Name() != "tester" {
		t.Fatalf("unexpected name: %s", c.Name())
	}
	owner := c.Owner()
	if owner == nil || owner.GetUID() != types.UID("TEST") {
		t.Fatalf("unexpected owner: %#v", owner)
	}
	if !c.HasParity() {
		t.Fatalf("expected parity")
	}
	if c.DB() == nil {
		t.Fatalf("expected db")
	}
}
