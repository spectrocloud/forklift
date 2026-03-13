package model

import "testing"

func TestPage_Slice_IgnoresNonPointer(t *testing.T) {
	p := &Page{Offset: 1, Limit: 1}
	s := []int{1, 2, 3}
	p.Slice(s) // should not panic or modify
	if len(s) != 3 {
		t.Fatalf("expected unchanged")
	}
}

func TestPage_Slice_IgnoresPointerToNonSlice(t *testing.T) {
	p := &Page{Offset: 1, Limit: 1}
	x := 10
	p.Slice(&x)
	if x != 10 {
		t.Fatalf("expected unchanged")
	}
}

func TestPage_Slice_OffsetAndLimit(t *testing.T) {
	p := &Page{Offset: 1, Limit: 2}
	s := []int{1, 2, 3, 4}
	p.Slice(&s)
	if len(s) != 2 || s[0] != 2 || s[1] != 3 {
		t.Fatalf("unexpected slice: %#v", s)
	}
}

func TestPage_Slice_OffsetBeyondLen_Empty(t *testing.T) {
	p := &Page{Offset: 10, Limit: 2}
	s := []int{1, 2, 3}
	p.Slice(&s)
	if len(s) != 0 {
		t.Fatalf("expected empty, got %#v", s)
	}
}

func TestPage_Slice_LimitZero_Empty(t *testing.T) {
	p := &Page{Offset: 0, Limit: 0}
	s := []int{1, 2, 3}
	p.Slice(&s)
	if len(s) != 0 {
		t.Fatalf("expected empty, got %#v", s)
	}
}

func TestBase_Pk_ReturnsPK(t *testing.T) {
	b := &Base{PK: "abc"}
	if b.Pk() != "abc" {
		t.Fatalf("expected pk abc, got %q", b.Pk())
	}
}

func TestPage_Slice_LimitGreaterThanLen_ReturnsToEnd(t *testing.T) {
	p := &Page{Offset: 2, Limit: 100}
	s := []int{1, 2, 3, 4}
	p.Slice(&s)
	if len(s) != 2 || s[0] != 3 || s[1] != 4 {
		t.Fatalf("unexpected slice: %#v", s)
	}
}
