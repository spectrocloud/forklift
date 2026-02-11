package gob

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

type ifaceM interface{ M() }

type implM struct {
	F32 float32
	C64 complex64
}

func (implM) M() {}

func TestEncoder_Encode_FloatAndComplexAndInterface(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	// Register concrete type for interface encoding.
	RegisterName("implM", implM{})

	type payload struct {
		F64  float64
		C128 complex128
		I    ifaceM
	}

	v := payload{
		F64:  1.25,
		C128: complex(2, -3),
		I:    implM{F32: 3.5, C64: complex(1, 2)},
	}
	if err := enc.Encode(v); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected bytes")
	}
}

func TestIsZero_CoversKinds(t *testing.T) {
	type S struct {
		A int
		B string
	}
	type H struct {
		I interface{}
		F func()
	}

	// Array.
	if !isZero(reflect.ValueOf([2]int{0, 0})) {
		t.Fatalf("expected array zero")
	}
	if isZero(reflect.ValueOf([2]int{0, 1})) {
		t.Fatalf("expected array non-zero")
	}

	// Map/slice/string.
	if !isZero(reflect.ValueOf(map[string]int{})) {
		t.Fatalf("expected map zero")
	}
	if !isZero(reflect.ValueOf([]int{})) {
		t.Fatalf("expected slice zero")
	}
	if !isZero(reflect.ValueOf("")) {
		t.Fatalf("expected string zero")
	}

	// Bool.
	if !isZero(reflect.ValueOf(false)) || isZero(reflect.ValueOf(true)) {
		t.Fatalf("unexpected bool zero")
	}

	// Complex.
	if !isZero(reflect.ValueOf(complex64(0))) || isZero(reflect.ValueOf(complex64(1+0i))) {
		t.Fatalf("unexpected complex64 zero")
	}

	// Pointer/interface/func.
	var p *int
	if !isZero(reflect.ValueOf(p)) {
		t.Fatalf("expected nil ptr zero")
	}
	hi := reflect.ValueOf(H{}).Field(0) // kind Interface, nil
	if !isZero(hi) {
		t.Fatalf("expected nil interface field zero")
	}
	hf := reflect.ValueOf(H{}).Field(1) // kind Func, nil
	if !isZero(hf) {
		t.Fatalf("expected nil func field zero")
	}

	// Struct.
	if !isZero(reflect.ValueOf(S{})) {
		t.Fatalf("expected struct zero")
	}
	if isZero(reflect.ValueOf(S{A: 1})) {
		t.Fatalf("expected struct non-zero")
	}
}

func TestEncHelpers_ArrayAndSliceHelpers_MoreKinds(t *testing.T) {
	enc := NewEncoder(io.Discard)
	b := new(encBuffer)
	state := enc.newEncoderState(b)
	state.sendZero = true

	type tc struct {
		name    string
		kind    reflect.Kind
		makeArr func() (any, reflect.Value) // returns (nonAddr, addr)
		makeSl  func() any
	}

	tests := []tc{
		{
			name: "int16",
			kind: reflect.Int16,
			makeArr: func() (any, reflect.Value) {
				a := [2]int16{0, 1}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []int16{0, 1} },
		},
		{
			name: "int32",
			kind: reflect.Int32,
			makeArr: func() (any, reflect.Value) {
				a := [2]int32{0, 1}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []int32{0, 1} },
		},
		{
			name: "int64",
			kind: reflect.Int64,
			makeArr: func() (any, reflect.Value) {
				a := [2]int64{0, 1}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []int64{0, 1} },
		},
		{
			name: "int8",
			kind: reflect.Int8,
			makeArr: func() (any, reflect.Value) {
				a := [2]int8{0, 1}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []int8{0, 1} },
		},
		{
			name: "uint",
			kind: reflect.Uint,
			makeArr: func() (any, reflect.Value) {
				a := [2]uint{0, 1}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []uint{0, 1} },
		},
		{
			name: "uint16",
			kind: reflect.Uint16,
			makeArr: func() (any, reflect.Value) {
				a := [2]uint16{0, 1}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []uint16{0, 1} },
		},
		{
			name: "uint64",
			kind: reflect.Uint64,
			makeArr: func() (any, reflect.Value) {
				a := [2]uint64{0, 1}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []uint64{0, 1} },
		},
		{
			name: "uintptr",
			kind: reflect.Uintptr,
			makeArr: func() (any, reflect.Value) {
				a := [2]uintptr{0, 1}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []uintptr{0, 1} },
		},
		{
			name: "float32",
			kind: reflect.Float32,
			makeArr: func() (any, reflect.Value) {
				a := [2]float32{0, 1.5}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []float32{0, 1.5} },
		},
		{
			name: "complex64",
			kind: reflect.Complex64,
			makeArr: func() (any, reflect.Value) {
				a := [2]complex64{0, 1 + 2i}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []complex64{0, 1 + 2i} },
		},
		{
			name: "complex128",
			kind: reflect.Complex128,
			makeArr: func() (any, reflect.Value) {
				a := [2]complex128{0, 1 + 2i}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []complex128{0, 1 + 2i} },
		},
		{
			name: "stringArray",
			kind: reflect.String,
			makeArr: func() (any, reflect.Value) {
				a := [2]string{"", "x"}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []string{"", "x"} },
		},
		{
			name: "boolArray",
			kind: reflect.Bool,
			makeArr: func() (any, reflect.Value) {
				a := [2]bool{false, true}
				return a, reflect.ValueOf(&a).Elem()
			},
			makeSl: func() any { return []bool{false, true} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Slice helper.
			sh := encSliceHelper[tt.kind]
			if sh == nil {
				t.Fatalf("missing slice helper for %v", tt.kind)
			}
			b.Reset()
			if ok := sh(state, reflect.ValueOf(tt.makeSl())); !ok {
				t.Fatalf("expected slice helper ok")
			}
			if b.Len() == 0 {
				t.Fatalf("expected bytes written")
			}

			// Array helper (direct function by kind).
			b.Reset()
			nonAddr, addr := tt.makeArr()
			var ok bool
			switch tt.kind {
			case reflect.Bool:
				ok = encBoolArray(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encBoolArray(state, addr)
			case reflect.Int8:
				ok = encInt8Array(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encInt8Array(state, addr)
			case reflect.Int16:
				ok = encInt16Array(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encInt16Array(state, addr)
			case reflect.Int32:
				ok = encInt32Array(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encInt32Array(state, addr)
			case reflect.Int64:
				ok = encInt64Array(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encInt64Array(state, addr)
			case reflect.Uint:
				ok = encUintArray(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encUintArray(state, addr)
			case reflect.Uint16:
				ok = encUint16Array(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encUint16Array(state, addr)
			case reflect.Uint64:
				ok = encUint64Array(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encUint64Array(state, addr)
			case reflect.Uintptr:
				ok = encUintptrArray(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encUintptrArray(state, addr)
			case reflect.Float32:
				ok = encFloat32Array(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encFloat32Array(state, addr)
			case reflect.Complex64:
				ok = encComplex64Array(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encComplex64Array(state, addr)
			case reflect.Complex128:
				ok = encComplex128Array(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encComplex128Array(state, addr)
			case reflect.String:
				ok = encStringArray(state, reflect.ValueOf(nonAddr))
				if ok {
					t.Fatalf("expected non-addressable false")
				}
				ok = encStringArray(state, addr)
			default:
				t.Fatalf("missing array helper switch for %v", tt.kind)
			}
			if !ok {
				t.Fatalf("expected addressable array ok")
			}
			if b.Len() == 0 {
				t.Fatalf("expected bytes written for array helper")
			}
		})
	}
}
