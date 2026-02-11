package gob

import (
	"reflect"
	"testing"
)

func TestEncHelpers_ArrayNotAddressable_ReturnsFalse(t *testing.T) {
	v := reflect.ValueOf([2]int{1, 2}) // not addressable
	if encArrayHelper[reflect.Int](nil, v) {
		t.Fatalf("expected false for non-addressable array")
	}
}

func TestEncHelpers_SliceWrongConcreteType_ReturnsFalse(t *testing.T) {
	type myInt int
	s := []myInt{1, 2}
	v := reflect.ValueOf(s)
	state := &encoderState{b: &encBuffer{}}
	if encSliceHelper[reflect.Int](state, v) {
		t.Fatalf("expected false for kind=int but not []int")
	}
}

func TestEncHelpers_SliceZerosAndSendZeroFalse_ReturnsTrueWithoutEncoding(t *testing.T) {
	s := []int{0, 0, 0}
	v := reflect.ValueOf(s)
	state := &encoderState{b: &encBuffer{}, sendZero: false}
	if !encSliceHelper[reflect.Int](state, v) {
		t.Fatalf("expected true")
	}
}
