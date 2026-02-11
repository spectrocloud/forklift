package gob

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestEncoder_EncodeNilValue_ReturnsError(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	err := enc.Encode(nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestEncoder_EncodeBasicTypes_WritesData(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	type sample struct {
		A int
		B string
	}

	values := []any{
		true,
		int(42),
		uint(7),
		"hello",
		[]byte{1, 2, 3},
		sample{A: 1, B: "x"},
		&sample{A: 2, B: "y"},
		map[string]int{"a": 1, "b": 2},
		[]string{"a", "b"},
	}
	for i, v := range values {
		if err := enc.Encode(v); err != nil {
			t.Fatalf("Encode[%d] (%T) failed: %v", i, v, err)
		}
	}

	if buf.Len() == 0 {
		t.Fatalf("expected encoded output, got empty buffer")
	}
}

type testGobEnc struct {
	called *bool
	data   []byte
	err    error
}

func (t testGobEnc) GobEncode() ([]byte, error) {
	if t.called != nil {
		*t.called = true
	}
	return t.data, t.err
}

func TestEncoder_EncodeGobEncoder_CallsGobEncode(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	called := false
	v := testGobEnc{called: &called, data: []byte("ok")}
	if err := enc.Encode(v); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if !called {
		t.Fatalf("expected GobEncode to be called")
	}
	if buf.Len() == 0 {
		t.Fatalf("expected encoded output, got empty buffer")
	}
}

func TestEncoder_EncodeGobEncoder_ErrorPropagates(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	wantErr := errors.New("boom")
	v := testGobEnc{data: []byte("ignored"), err: wantErr}
	if err := enc.Encode(v); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestRegisterName_DuplicateTypesPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic, got nil")
		}
	}()

	type a struct{ X int }
	type b struct{ X int }

	RegisterName("dup", a{})
	RegisterName("dup", b{})
}

func TestEncHelpers_SliceHelpers_EncodeCommonKinds(t *testing.T) {
	enc := NewEncoder(io.Discard)
	b := new(encBuffer)
	state := enc.newEncoderState(b)
	state.sendZero = true

	tests := []struct {
		name      string
		kind      reflect.Kind
		value     any
		aliasFail any
	}{
		{
			name:      "bool",
			kind:      reflect.Bool,
			value:     []bool{false, true},
			aliasFail: []typeBool{true},
		},
		{
			name:      "int",
			kind:      reflect.Int,
			value:     []int{0, 1, -2},
			aliasFail: []typeInt{1},
		},
		{
			name:      "string",
			kind:      reflect.String,
			value:     []string{"", "x"},
			aliasFail: []typeString{"x"},
		},
		{
			name:      "float64",
			kind:      reflect.Float64,
			value:     []float64{0, 1.25},
			aliasFail: []typeFloat64{1.25},
		},
		{
			name:      "uint32",
			kind:      reflect.Uint32,
			value:     []uint32{0, 7},
			aliasFail: []typeUint32{7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := encSliceHelper[tt.kind]
			if helper == nil {
				t.Fatalf("missing slice helper for kind %v", tt.kind)
			}

			// success path
			b.Reset()
			ok := helper(state, reflect.ValueOf(tt.value))
			if !ok {
				t.Fatalf("expected ok=true")
			}
			if b.Len() == 0 {
				t.Fatalf("expected some encoded bytes")
			}

			// alias type should fail Interface().([]T) assertion.
			b.Reset()
			ok = helper(state, reflect.ValueOf(tt.aliasFail))
			if ok {
				t.Fatalf("expected ok=false for alias type")
			}
			if b.Len() != 0 {
				t.Fatalf("expected no bytes written for failed helper")
			}
		})
	}
}

func TestEncHelpers_ArrayHelpers_AddressableRequirement(t *testing.T) {
	enc := NewEncoder(io.Discard)
	b := new(encBuffer)
	state := enc.newEncoderState(b)
	state.sendZero = true

	arr := [2]int{0, 1}
	// Not addressable.
	if ok := encIntArray(state, reflect.ValueOf(arr)); ok {
		t.Fatalf("expected non-addressable array to return false")
	}
	// Addressable.
	b.Reset()
	v := reflect.ValueOf(&arr).Elem()
	if ok := encIntArray(state, v); !ok {
		t.Fatalf("expected addressable array to return true")
	}
	if b.Len() == 0 {
		t.Fatalf("expected some encoded bytes")
	}
}

// Alias types for negative tests (should not satisfy []T assertions).
type typeBool bool
type typeInt int
type typeString string
type typeFloat64 float64
type typeUint32 uint32
