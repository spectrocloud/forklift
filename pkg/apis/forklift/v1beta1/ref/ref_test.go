package ref

import "testing"

func TestRef_NotSetAndString(t *testing.T) {
	var r Ref
	if !r.NotSet() {
		t.Fatalf("expected NotSet for empty ref")
	}

	r = Ref{ID: "id1", Name: "n1"}
	if r.NotSet() {
		t.Fatalf("expected NotSet=false when ID/Name set")
	}
	if s := r.String(); s == "" {
		t.Fatalf("expected non-empty string")
	}

	r = Ref{Type: "VM", ID: "id1"}
	if s := r.String(); s == "" || s[0] != '(' {
		t.Fatalf("expected typed string, got %q", s)
	}
}

func TestRefs_Find(t *testing.T) {
	rr := &Refs{
		List: []Ref{{ID: "a"}, {ID: "b"}},
	}
	if !rr.Find(Ref{ID: "b"}) {
		t.Fatalf("expected to find ref")
	}
	if rr.Find(Ref{ID: "c"}) {
		t.Fatalf("expected not to find ref")
	}
}

func TestGeneratedDeepCopy_RefAndRefs(t *testing.T) {
	in := &Ref{ID: "id1", Name: "n1", Namespace: "ns", Type: "VM"}
	cp := in.DeepCopy()
	if cp == nil || cp == in || cp.ID != "id1" || cp.Namespace != "ns" {
		t.Fatalf("unexpected deepcopy: %#v", cp)
	}

	refs := &Refs{List: []Ref{{ID: "a"}, {ID: "b"}}}
	refsCopy := refs.DeepCopy()
	if refsCopy == nil || refsCopy == refs || len(refsCopy.List) != 2 {
		t.Fatalf("unexpected deepcopy: %#v", refsCopy)
	}
	refsCopy.List[0].ID = "changed"
	if refs.List[0].ID != "a" {
		t.Fatalf("expected list slice deep-copied")
	}
}
