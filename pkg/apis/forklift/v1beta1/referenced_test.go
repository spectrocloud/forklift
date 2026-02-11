package v1beta1

import "testing"

func TestReferenced_DeepCopyInto_NoPanic(t *testing.T) {
	in := &Referenced{}
	out := &Referenced{}
	in.DeepCopyInto(out)
}

func TestReferenced_DeepCopy_ReturnsSamePointer(t *testing.T) {
	in := &Referenced{}
	if got := in.DeepCopy(); got != in {
		t.Fatalf("expected DeepCopy to return same pointer")
	}
}
