package filebacked

import (
	"os"
	"path/filepath"
	"testing"
)

type fbPerson struct {
	ID   int
	Name string
}

func withTempWorkingDir(t *testing.T) func() {
	t.Helper()
	old := WorkingDir
	WorkingDir = t.TempDir()
	return func() { WorkingDir = old }
}

func withCatalogSnapshot(t *testing.T) func() {
	t.Helper()
	catalog.Lock()
	old := append([]interface{}(nil), catalog.content...)
	catalog.Unlock()
	return func() {
		catalog.Lock()
		catalog.content = old
		catalog.Unlock()
	}
}

func TestEmptyIterator_LenIsZero(t *testing.T) {
	itr := &EmptyIterator{}
	if itr.Len() != 0 {
		t.Fatalf("expected 0")
	}
}

func TestEmptyIterator_NextFalse(t *testing.T) {
	itr := &EmptyIterator{}
	obj, ok := itr.Next()
	if ok || obj != nil {
		t.Fatalf("expected (nil,false)")
	}
}

func TestEmptyIterator_NextWithFalse(t *testing.T) {
	itr := &EmptyIterator{}
	if itr.NextWith(&fbPerson{}) {
		t.Fatalf("expected false")
	}
}

func TestEmptyIterator_AtNil(t *testing.T) {
	itr := &EmptyIterator{}
	if itr.At(0) != nil {
		t.Fatalf("expected nil")
	}
}

func TestEmptyIterator_AtWith_NoPanic(t *testing.T) {
	itr := &EmptyIterator{}
	itr.AtWith(0, &fbPerson{})
}

func TestEmptyIterator_Reverse_NoPanic(t *testing.T) {
	itr := &EmptyIterator{}
	itr.Reverse()
}

func TestEmptyIterator_Close_NoPanic(t *testing.T) {
	itr := &EmptyIterator{}
	itr.Close()
}

func TestWriter_Close_ZeroValue_NoPanic(t *testing.T) {
	var w Writer
	w.Close()
}

func TestWriter_Append_CreatesFileInWorkingDir(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1, Name: "a"})
	if w.path == "" {
		t.Fatalf("expected path")
	}
	if filepath.Dir(w.path) != WorkingDir {
		t.Fatalf("expected in working dir")
	}
	if _, err := os.Stat(w.path); err != nil {
		t.Fatalf("expected file exists: %v", err)
	}
	w.Close()
}

func TestWriter_Append_TwoObjects_IncreasesIndexLen(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	w.Append(&fbPerson{ID: 2})
	if len(w.index) != 2 {
		t.Fatalf("expected 2 got %d", len(w.index))
	}
	w.Close()
}

func TestWriter_Reader_SharedFalse_CreatesLinkedPath(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(false)
	defer r.Close()
	if r.path == "" || r.path == w.path {
		t.Fatalf("expected linked path")
	}
	if _, err := os.Stat(r.path); err != nil {
		t.Fatalf("expected link exists: %v", err)
	}
	w.Close()
}

func TestWriter_Reader_SharedFalse_CloseRemovesLinkedFile(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(false)
	path := r.path
	r.Close()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected removed")
	}
	w.Close()
}

func TestWriter_Reader_SharedFalse_DoesNotRemoveWriterFile(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(false)
	r.Close()
	if _, err := os.Stat(w.path); err != nil {
		t.Fatalf("expected writer file remains until writer close: %v", err)
	}
	w.Close()
}

func TestReader_Len_MatchesIndexLen(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	w.Append(&fbPerson{ID: 2})
	r := w.Reader(false)
	defer r.Close()
	if r.Len() != 2 {
		t.Fatalf("expected 2")
	}
	w.Close()
}

func TestReader_At_DecodesPointerObject(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 7, Name: "x"})
	r := w.Reader(false)
	defer r.Close()
	obj := r.At(0)
	p, ok := obj.(*fbPerson)
	if !ok {
		t.Fatalf("expected *fbPerson got %T", obj)
	}
	if p.ID != 7 || p.Name != "x" {
		t.Fatalf("unexpected: %#v", p)
	}
	w.Close()
}

func TestReader_AtWith_DecodesIntoProvidedStruct(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 9, Name: "y"})
	r := w.Reader(false)
	defer r.Close()
	var out fbPerson
	r.AtWith(0, &out)
	if out.ID != 9 || out.Name != "y" {
		t.Fatalf("unexpected out: %#v", out)
	}
	w.Close()
}

func TestReader_Close_ZeroValue_NoPanic(t *testing.T) {
	var r Reader
	r.Close()
}

func TestReader_Open_LazyOpensFile(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(false)
	defer r.Close()
	if r.file != nil {
		t.Fatalf("expected lazy nil file")
	}
	_ = r.At(0)
	if r.file == nil {
		t.Fatalf("expected opened file")
	}
	w.Close()
}

func TestList_Iter_Empty_ReturnsEmptyIterator(t *testing.T) {
	l := NewList()
	defer l.Close()
	itr := l.Iter()
	if itr.Len() != 0 {
		t.Fatalf("expected 0")
	}
	if _, ok := itr.Next(); ok {
		t.Fatalf("expected no next")
	}
}

func TestWriter_Close_RemovesWriterFile(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	path := w.path
	w.Close()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected removed")
	}
}

func TestWriter_Append_DoesNotChangePathAfterFirstOpen(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	p1 := w.path
	w.Append(&fbPerson{ID: 2})
	if w.path != p1 {
		t.Fatalf("expected same path")
	}
	w.Close()
}

func TestWriter_Reader_SharedTrue_UsesSamePath(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(true)
	if r.path != w.path {
		t.Fatalf("expected same path")
	}
	// Avoid calling r.Close() because shared is not marked; writer owns file lifecycle.
	w.Close()
}

func TestWriter_Reader_SharedTrue_SharesFilePointer(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(true)
	if r.file != w.file {
		t.Fatalf("expected shared file pointer")
	}
	w.Close()
}

func TestWriter_Reader_SharedTrue_LenMatches(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	w.Append(&fbPerson{ID: 2})
	r := w.Reader(true)
	if r.Len() != 2 {
		t.Fatalf("expected 2")
	}
	w.Close()
}

func TestWriter_Dirty_TrueAfterAppend_FalseAfterReader(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	if !w.dirty {
		t.Fatalf("expected dirty")
	}
	_ = w.Reader(true) // flush called
	if w.dirty {
		t.Fatalf("expected clean after flush")
	}
	w.Close()
}

func TestReader_Close_RemovesPathWhenFileNil(t *testing.T) {
	defer withTempWorkingDir(t)()
	// Create a dummy file for the reader to remove.
	p := filepath.Join(WorkingDir, "x.fb")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := &Reader{path: p, shared: false, file: nil}
	r.Close()
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("expected removed")
	}
}

func TestReader_Close_SharedTrue_DoesNothing(t *testing.T) {
	defer withTempWorkingDir(t)()
	p := filepath.Join(WorkingDir, "x.fb")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := &Reader{path: p, shared: true}
	r.Close()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected file remains")
	}
	_ = os.Remove(p)
}

func TestReader_readEntry_AtEOF_ReturnsZeroKindNilBuf(t *testing.T) {
	defer withTempWorkingDir(t)()
	f, err := os.Create(filepath.Join(WorkingDir, "x.fb"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	r := &Reader{file: f}
	kind, b := r.readEntry()
	if kind != 0 || b != nil {
		t.Fatalf("expected (0,nil), got (%d,%v)", kind, b)
	}
	_ = f.Close()
}

func TestReader_open_DoesNotReopenWhenFileAlreadySet(t *testing.T) {
	defer withTempWorkingDir(t)()
	f, err := os.Create(filepath.Join(WorkingDir, "x.fb"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	r := &Reader{path: f.Name(), file: f}
	r.open()
	if r.file != f {
		t.Fatalf("expected same file")
	}
	_ = f.Close()
}

func TestFbIterator_Next_ExhaustedReturnsFalse(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	list := NewList()
	defer list.Close()
	list.Append(&fbPerson{ID: 1})
	itr := list.Iter()
	_, ok := itr.Next()
	if !ok {
		t.Fatalf("expected ok")
	}
	_, ok = itr.Next()
	if ok {
		t.Fatalf("expected exhausted")
	}
	itr.Close()
}

func TestFbIterator_NextWith_FillsStruct(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	list := NewList()
	defer list.Close()
	list.Append(&fbPerson{ID: 3, Name: "z"})
	itr := list.Iter()
	var out fbPerson
	if !itr.NextWith(&out) {
		t.Fatalf("expected true")
	}
	if out.ID != 3 || out.Name != "z" {
		t.Fatalf("unexpected: %#v", out)
	}
	itr.Close()
}

func TestFbIterator_NextWith_ExhaustedFalse(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	list := NewList()
	defer list.Close()
	list.Append(&fbPerson{ID: 1})
	itr := list.Iter()
	_ = itr.NextWith(&fbPerson{})
	if itr.NextWith(&fbPerson{}) {
		t.Fatalf("expected false")
	}
	itr.Close()
}

func TestFbIterator_Reverse_Empty_NoPanic(t *testing.T) {
	itr := &FbIterator{Reader: &Reader{index: []int64{}}}
	itr.Reverse()
}

func TestFbIterator_Reverse_ReversesIndex(t *testing.T) {
	itr := &FbIterator{Reader: &Reader{index: []int64{1, 2, 3}}}
	itr.Reverse()
	if itr.index[0] != 3 || itr.index[1] != 2 || itr.index[2] != 1 {
		t.Fatalf("unexpected index: %#v", itr.index)
	}
}

func TestFbIterator_Next_IncrementsCurrent(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	list := NewList()
	defer list.Close()
	list.Append(&fbPerson{ID: 1})
	list.Append(&fbPerson{ID: 2})
	itr := list.Iter().(*FbIterator)
	if itr.current != 0 {
		t.Fatalf("expected current=0")
	}
	_, ok := itr.Next()
	if !ok || itr.current != 1 {
		t.Fatalf("expected current=1")
	}
	itr.Close()
}

func TestFbIterator_NextWith_IncrementsCurrent(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	list := NewList()
	defer list.Close()
	list.Append(&fbPerson{ID: 1})
	itr := list.Iter().(*FbIterator)
	if itr.current != 0 {
		t.Fatalf("expected current=0")
	}
	ok := itr.NextWith(&fbPerson{})
	if !ok || itr.current != 1 {
		t.Fatalf("expected current=1")
	}
	itr.Close()
}

func TestReader_Len_ZeroWhenNilIndex(t *testing.T) {
	r := &Reader{}
	if r.Len() != 0 {
		t.Fatalf("expected 0")
	}
}

func TestWriter_Reader_SharedFalse_IndexIsSnapshotLen_CurrentBehavior(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(false)
	defer r.Close()
	// Current behavior: r.index is w.index[:] (shares backing array),
	// but Len() is a snapshot of the slice length at creation time.
	w.Append(&fbPerson{ID: 2})
	if r.Len() != 1 {
		t.Fatalf("expected snapshot len=1, got %d", r.Len())
	}
	w.Close()
}

func TestList_Close_ZeroValue_NoPanic(t *testing.T) {
	var l List
	l.Close()
}

func TestList_Len_ZeroOnNew(t *testing.T) {
	l := NewList()
	defer l.Close()
	if l.Len() != 0 {
		t.Fatalf("expected 0")
	}
}

func TestList_At_PanicsWhenEmpty(t *testing.T) {
	l := NewList()
	defer l.Close()
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	_ = l.At(0)
}

// ---- Consolidated from iterator_more_test.go ----

func TestReader_At_IndexOutOfRange_Panics(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(false)
	defer r.Close()
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	_ = r.At(1)
	w.Close()
}

func TestReader_AtWith_IndexOutOfRange_Panics(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(false)
	defer r.Close()
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	var out fbPerson
	r.AtWith(1, &out)
	w.Close()
}

func TestReader_Close_Twice_NoPanic(t *testing.T) {
	defer withTempWorkingDir(t)()
	p := filepath.Join(WorkingDir, "x.fb")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := &Reader{path: p}
	r.Close()
	r.Close()
}

func TestWriter_Close_Twice_NoPanic(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	w.Close()
	w.Close()
}

func TestList_Close_Twice_NoPanic(t *testing.T) {
	l := NewList()
	l.Close()
	l.Close()
}

func TestList_Iter_NonEmpty_ReturnsFbIterator(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	l := NewList()
	defer l.Close()
	l.Append(&fbPerson{ID: 1})
	itr := l.Iter()
	if _, ok := itr.(*FbIterator); !ok {
		t.Fatalf("expected *FbIterator, got %T", itr)
	}
	itr.Close()
}

func TestFbIterator_Reverse_ChangesNextOrder(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	l := NewList()
	defer l.Close()
	l.Append(&fbPerson{ID: 1})
	l.Append(&fbPerson{ID: 2})
	l.Append(&fbPerson{ID: 3})
	itr := l.Iter().(*FbIterator)
	itr.Reverse()
	a, _ := itr.Next()
	b, _ := itr.Next()
	c, _ := itr.Next()
	if a.(*fbPerson).ID != 3 || b.(*fbPerson).ID != 2 || c.(*fbPerson).ID != 1 {
		t.Fatalf("unexpected reverse order")
	}
	itr.Close()
}

func TestFbIterator_Reverse_ThenNextWith_ChangesDecodeOrder(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	l := NewList()
	defer l.Close()
	l.Append(&fbPerson{ID: 1})
	l.Append(&fbPerson{ID: 2})
	itr := l.Iter().(*FbIterator)
	itr.Reverse()
	var out1, out2 fbPerson
	_ = itr.NextWith(&out1)
	_ = itr.NextWith(&out2)
	if out1.ID != 2 || out2.ID != 1 {
		t.Fatalf("unexpected: %#v %#v", out1, out2)
	}
	itr.Close()
}

func TestReader_open_WhenSharedTrue_DoesNothing(t *testing.T) {
	r := &Reader{shared: true, path: "/does-not-exist"}
	// Should not attempt os.Open.
	r.open()
}

func TestWriter_Reader_SharedFalse_FileInitiallyNil(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(false)
	defer r.Close()
	if r.file != nil {
		t.Fatalf("expected nil file (lazy open)")
	}
	w.Close()
}

func TestReader_At_OpensFileOnce(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(false)
	defer r.Close()
	_ = r.At(0)
	f := r.file
	_ = r.At(0)
	if r.file != f {
		t.Fatalf("expected same file")
	}
	w.Close()
}

func TestReader_AtWith_OpensFileOnce(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(false)
	defer r.Close()
	var out fbPerson
	r.AtWith(0, &out)
	f := r.file
	r.AtWith(0, &out)
	if r.file != f {
		t.Fatalf("expected same file")
	}
	w.Close()
}

func TestWriter_Reader_SharedFalse_PathHasExtension(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(false)
	defer r.Close()
	if filepath.Ext(r.path) != Extension {
		t.Fatalf("expected %s extension", Extension)
	}
	w.Close()
}

func TestWriter_PathHasExtension(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	if filepath.Ext(w.path) != Extension {
		t.Fatalf("expected %s extension", Extension)
	}
	w.Close()
}

func TestWriter_Append_ThenReaderAtWith_DecodeMatches(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 5, Name: "n"})
	r := w.Reader(false)
	defer r.Close()
	var out fbPerson
	r.AtWith(0, &out)
	if out.ID != 5 || out.Name != "n" {
		t.Fatalf("unexpected: %#v", out)
	}
	w.Close()
}

func TestList_AtWith_PanicsWhenEmpty(t *testing.T) {
	l := NewList()
	defer l.Close()
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	var out fbPerson
	l.AtWith(0, &out)
}

func TestList_Iter_EmptyIterator_Close_NoPanic(t *testing.T) {
	l := NewList()
	defer l.Close()
	itr := l.Iter()
	itr.Close()
}

func TestEmptyIterator_NextAlwaysFalse(t *testing.T) {
	itr := &EmptyIterator{}
	for i := 0; i < 3; i++ {
		_, ok := itr.Next()
		if ok {
			t.Fatalf("expected false")
		}
	}
}

func TestEmptyIterator_NextWithAlwaysFalse(t *testing.T) {
	itr := &EmptyIterator{}
	for i := 0; i < 3; i++ {
		if itr.NextWith(&fbPerson{}) {
			t.Fatalf("expected false")
		}
	}
}

func TestFbIterator_Close_NoPanic(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	l := NewList()
	defer l.Close()
	l.Append(&fbPerson{ID: 1})
	itr := l.Iter().(*FbIterator)
	itr.Close()
}

func TestList_Append_Iterator_CopiesAllItems(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	a := NewList()
	defer a.Close()
	a.Append(&fbPerson{ID: 1})
	a.Append(&fbPerson{ID: 2})
	b := NewList()
	defer b.Close()
	b.Append(a.Iter())
	if b.Len() != a.Len() {
		t.Fatalf("expected same len")
	}
}

func TestList_Append_Iterator_OnEmptySource_NoChange(t *testing.T) {
	a := NewList()
	defer a.Close()
	b := NewList()
	defer b.Close()
	b.Append(a.Iter())
	if b.Len() != 0 {
		t.Fatalf("expected 0")
	}
}

func TestList_Iter_FbIterator_CloseRemovesLinkedFile(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	l := NewList()
	defer l.Close()
	l.Append(&fbPerson{ID: 1})
	itr := l.Iter().(*FbIterator)
	path := itr.Reader.path
	itr.Close()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected removed")
	}
}

func TestList_AtWith_NonEmpty_Decodes(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	l := NewList()
	defer l.Close()
	l.Append(&fbPerson{ID: 7, Name: "a"})
	var out fbPerson
	l.AtWith(0, &out)
	if out.ID != 7 || out.Name != "a" {
		t.Fatalf("unexpected: %#v", out)
	}
}

func TestList_At_NonEmpty_ReturnsPointer(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	l := NewList()
	defer l.Close()
	l.Append(&fbPerson{ID: 7})
	obj := l.At(0)
	if _, ok := obj.(*fbPerson); !ok {
		t.Fatalf("expected *fbPerson got %T", obj)
	}
}

func TestReader_Open_WhenSharedTrueAndFileNil_NoPanic(t *testing.T) {
	r := &Reader{shared: true, path: "/does-not-exist", file: nil}
	r.open()
}

func TestReader_Close_WhenSharedTrueAndFileSet_NoPanic(t *testing.T) {
	defer withTempWorkingDir(t)()
	f, err := os.Create(filepath.Join(WorkingDir, "x.fb"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	r := &Reader{shared: true, file: f, path: f.Name()}
	r.Close()
	_ = f.Close()
	_ = os.Remove(f.Name())
}

func TestWriter_Reader_SharedTrue_AtWorks(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(true)
	obj := r.At(0)
	if obj.(*fbPerson).ID != 1 {
		t.Fatalf("expected id=1")
	}
	w.Close()
}

func TestWriter_Reader_SharedTrue_AtWithWorks(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 2, Name: "n"})
	r := w.Reader(true)
	var out fbPerson
	r.AtWith(0, &out)
	if out.ID != 2 || out.Name != "n" {
		t.Fatalf("unexpected: %#v", out)
	}
	w.Close()
}

func TestWriter_Reader_SharedTrue_DoesNotSetSharedFlag_CurrentBehavior(t *testing.T) {
	defer withTempWorkingDir(t)()
	defer withCatalogSnapshot(t)()
	var w Writer
	w.Append(&fbPerson{ID: 1})
	r := w.Reader(true)
	if r.shared {
		t.Fatalf("expected shared flag false (current behavior)")
	}
	w.Close()
}
