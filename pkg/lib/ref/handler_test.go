package ref

import (
	"testing"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ownerObj struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func TestToKind_ReturnsTypeSuffix(t *testing.T) {
	kind := ToKind(&core.Pod{})
	if kind != "Pod" {
		t.Fatalf("expected Pod got %q", kind)
	}
}

func TestGetRequests_EmptyWhenNoMapping(t *testing.T) {
	// Ensure clean map.
	Map = &RefMap{Content: map[Target]map[Owner]bool{}}
	a := &core.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "s"}}
	reqs := GetRequests("Owner", a)
	if len(reqs) != 0 {
		t.Fatalf("expected empty")
	}
}

func TestGetRequests_FiltersByOwnerKind(t *testing.T) {
	Map = &RefMap{Content: map[Target]map[Owner]bool{}}
	target := Target{Kind: "Secret", Namespace: "ns", Name: "s"}
	Map.Content[target] = map[Owner]bool{
		{Kind: "A", Namespace: "ns", Name: "o1"}: true,
		{Kind: "B", Namespace: "ns", Name: "o2"}: true,
	}

	a := &core.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "s"}}
	reqs := GetRequests("A", a)
	if len(reqs) != 1 {
		t.Fatalf("expected 1 got %d", len(reqs))
	}
	if reqs[0].Namespace != "ns" || reqs[0].Name != "o1" {
		t.Fatalf("unexpected req: %#v", reqs[0])
	}
}

// ---- Consolidated from ref_more_test.go ----

func TestRefSet(t *testing.T) {
	if RefSet(nil) {
		t.Fatalf("expected false")
	}
	if RefSet(&core.ObjectReference{}) {
		t.Fatalf("expected false")
	}
	if RefSet(&core.ObjectReference{Namespace: "ns"}) {
		t.Fatalf("expected false")
	}
	if RefSet(&core.ObjectReference{Namespace: "ns", Name: "n"}) != true {
		t.Fatalf("expected true")
	}
}

func TestDeepEquals(t *testing.T) {
	a := &core.ObjectReference{Namespace: "ns", Name: "n"}
	b := &core.ObjectReference{Namespace: "ns", Name: "n"}
	if !DeepEquals(a, b) {
		t.Fatalf("expected true")
	}
	if DeepEquals(a, nil) {
		t.Fatalf("expected false")
	}
}

func TestEquals(t *testing.T) {
	a := &core.ObjectReference{Namespace: "ns", Name: "n"}
	b := &core.ObjectReference{Namespace: "ns", Name: "n"}
	if !Equals(a, b) {
		t.Fatalf("expected true")
	}
	if Equals(a, &core.ObjectReference{Namespace: "ns", Name: "x"}) {
		t.Fatalf("expected false")
	}
	if !Equals(nil, nil) {
		t.Fatalf("expected true for nil,nil")
	}
	if Equals(a, nil) {
		t.Fatalf("expected false for non-nil,nil")
	}
}
