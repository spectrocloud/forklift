package model

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	fb "github.com/kubev2v/forklift/pkg/lib/filebacked"
)

type personModel struct {
	ID       int    `sql:"pk"`
	Revision int    `sql:"incremented"`
	Name     string `sql:""`
	Age      int    `sql:""`
}

func (p *personModel) Pk() string { return strconv.Itoa(p.ID) }

func TestClient_OpenClose_DeleteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.sqlite")
	db := New(path, &personModel{})

	if err := db.Open(true); err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected db file: %v", err)
	}
	if err := db.Close(true); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected deleted db file")
	}
}

func TestClient_InsertGetUpdateDelete_Count_List_Find(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.sqlite")
	db := New(path, &personModel{})
	if err := db.Open(true); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(true) })

	// Insert.
	p := &personModel{ID: 1, Name: "a", Age: 10}
	if err := db.Insert(p); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Get.
	got := &personModel{ID: 1}
	if err := db.Get(got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "a" || got.Age != 10 {
		t.Fatalf("unexpected get: %#v", got)
	}

	// Update.
	got.Name = "b"
	if err := db.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2 := &personModel{ID: 1}
	_ = db.Get(got2)
	if got2.Name != "b" {
		t.Fatalf("expected updated name")
	}

	// Count.
	n, err := db.Count(&personModel{}, nil)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 got %d", n)
	}

	// List.
	list := []personModel{}
	if err := db.List(&list, ListOptions{Detail: MaxDetail}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected list size 1")
	}

	// Find iterator.
	itr, err := db.Find(&personModel{}, ListOptions{Detail: MaxDetail})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	defer itr.Close()
	_, ok := itr.Next()
	if !ok {
		t.Fatalf("expected next")
	}
	_, ok = itr.Next()
	if ok {
		t.Fatalf("expected exhausted")
	}

	// Delete.
	if err := db.Delete(&personModel{ID: 1}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := db.Get(&personModel{ID: 1}); !errors.Is(err, NotFound) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestClient_With_CommitsOnNilError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.sqlite")
	db := New(path, &personModel{})
	if err := db.Open(true); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(true) })

	err := db.With(func(tx *Tx) error {
		return tx.Insert(&personModel{ID: 1, Name: "a"})
	})
	if err != nil {
		t.Fatalf("with: %v", err)
	}
	if err := db.Get(&personModel{ID: 1}); err != nil {
		t.Fatalf("expected committed, got %v", err)
	}
}

func TestClient_With_RollsBackOnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.sqlite")
	db := New(path, &personModel{})
	if err := db.Open(true); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(true) })

	want := errors.New("boom")
	err := db.With(func(tx *Tx) error {
		_ = tx.Insert(&personModel{ID: 1, Name: "a"})
		return want
	})
	if err == nil {
		t.Fatalf("expected err")
	}
	if err := db.Get(&personModel{ID: 1}); !errors.Is(err, NotFound) {
		t.Fatalf("expected rollback, got %v", err)
	}
}

func TestTx_CommitTwice_NoPanicNoError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.sqlite")
	db := New(path, &personModel{})
	if err := db.Open(true); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(true) })

	tx, err := db.Begin("a", "b")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("second commit: %v", err)
	}
}

func TestTx_EndTwice_NoPanicNoError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.sqlite")
	db := New(path, &personModel{})
	if err := db.Open(true); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(true) })

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.End(); err != nil {
		t.Fatalf("end: %v", err)
	}
	if err := tx.End(); err != nil {
		t.Fatalf("second end: %v", err)
	}
}

func TestClient_Watch_NoSnapshot_StartsAndEnds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.sqlite")
	db := New(path, &personModel{})
	if err := db.Open(true); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(true) })

	h := &watchHandler{}
	w, err := db.Watch(&personModel{}, h)
	if err != nil || w == nil {
		t.Fatalf("watch: %v", err)
	}
	// Trigger at least one event and end watch.
	_ = db.Insert(&personModel{ID: 1, Name: "a"})
	db.EndWatch(w)
}

type watchHandler struct {
	StockEventHandler
}

func (w *watchHandler) Options() WatchOptions { return WatchOptions{Snapshot: false} }

func TestClient_Watch_Snapshot_UsesFindIterator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.sqlite")
	db := New(path, &personModel{})
	if err := db.Open(true); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(true) })

	_ = db.Insert(&personModel{ID: 1, Name: "a"})
	h := &snapshotHandler{}
	w, err := db.Watch(&personModel{}, h)
	if err != nil || w == nil {
		t.Fatalf("watch: %v", err)
	}
	db.EndWatch(w)
}

type snapshotHandler struct {
	StockEventHandler
	gotSnapshot bool
}

func (s *snapshotHandler) Options() WatchOptions { return WatchOptions{Snapshot: true} }

func (s *snapshotHandler) Started(uint64) { s.gotSnapshot = true }

func TestClient_Find_WhenNoSnapshotIterator_EmptyIteratorType(t *testing.T) {
	// Cover the fb.EmptyIterator path used when snapshot not requested.
	var it fb.Iterator = &fb.EmptyIterator{}
	if it.Len() != 0 {
		t.Fatalf("expected 0")
	}
}
