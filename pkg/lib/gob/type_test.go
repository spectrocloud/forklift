package gob

import (
	"encoding"
	"reflect"
	"testing"
)

type recursivePtr *recursivePtr

type myGob struct{}

func (myGob) GobEncode() ([]byte, error) { return []byte("x"), nil }
func (*myGob) GobDecode([]byte) error    { return nil }

func TestImplementsInterface_NilType(t *testing.T) {
	if ok, _ := implementsInterface(nil, gobEncoderInterfaceType); ok {
		t.Fatalf("expected false")
	}
}

func TestImplementsInterface_PointerIndirections(t *testing.T) {
	// myGob implements GobEncoder (value receiver) and GobDecoder (pointer receiver).
	typ := reflect.TypeOf(myGob{})
	if ok, indir := implementsInterface(typ, gobEncoderInterfaceType); !ok || indir != 0 {
		t.Fatalf("expected ok indir=0, got ok=%v indir=%d", ok, indir)
	}
	if ok, indir := implementsInterface(typ, gobDecoderInterfaceType); !ok {
		t.Fatalf("expected ok for decoder, got ok=%v indir=%d", ok, indir)
	}
}

func TestValidUserType_RecursivePointerTypeErrors(t *testing.T) {
	// This creates a pointer-cycle type (T = *T).
	rt := reflect.TypeOf((*recursivePtr)(nil)).Elem()
	if _, err := validUserType(rt); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidUserType_Caches(t *testing.T) {
	rt := reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	// First call: computes and stores.
	_, _ = validUserType(rt)
	// Second call: hits cache.
	if _, err := validUserType(rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
