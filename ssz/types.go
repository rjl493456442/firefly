package ssz

import (
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"sync"
)

// isUint returns whether the given type belongs to uintN where N in [8, 16, 32, 64].
//
// We don't support uint since the size of uint depends on the type of architecture
// you are using.
func isUint(t reflect.Type) bool {
	return t.Kind() >= reflect.Uint8 && t.Kind() <= reflect.Uint64
}

func isByte(typ reflect.Type) bool {
	return typ.Kind() == reflect.Uint8 && !typ.Implements(encoderInterface)
}

// isFixedType returns true if the type is fixed-size.
// The following types are called `Fixed`:
// * bool
// * uintN: N-bit unsigned integer (where N in [8, 16, 32, 64, 128, 256])
// * T[k] for any fixed element T
// * (T1,...,Tk) if all T are fixed.
func isFixedType(typ reflect.Type) bool {
	kind := typ.Kind()
	switch {
	case kind == reflect.Interface:
		return isFixedType(typ.Elem())
	case kind == reflect.Ptr:
		return isFixedType(typ.Elem())
	case kind == reflect.Bool || isUint(typ):
		return true
	case kind == reflect.Array && isByte(typ.Elem()):
		return true
	case kind == reflect.Slice && isByte(typ.Elem()):
		return false
	case kind == reflect.String:
		return false
	case kind == reflect.Array && typ.Elem().Kind() != reflect.Interface:
		return isFixedType(typ.Elem())
	case kind == reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			if !isFixedType(typ.Field(i).Type) {
				return false
			}
		}
		return true
	case kind == reflect.Slice:
		return false
	default:
		return false
	}
}

// getTypeSize returns the size that this type needs to occupy.
//
// We distinguish fixed-size and variable-size types. Fixed-size types are encoded
// in-place and variable-size types are encoded at a separately allocated location
// after the current block.
//
// So for a fixed-size type, the size returned represents the size that the variable
// actually occupies.
// For a variable-size type, the returned size is fixed 4 bytes, which is used to
// store the location reference for actual value storage.
func getTypeSize(v reflect.Value) int {
	kind := v.Kind()
	switch {
	case kind == reflect.Interface:
		return getTypeSize(v.Elem())
	case kind == reflect.Ptr:
		return getTypeSize(v.Elem())
	case kind == reflect.Bool || kind == reflect.Uint8:
		return 1
	case kind == reflect.Uint16:
		return 2
	case kind == reflect.Uint32:
		return 4
	case kind == reflect.Uint64:
		return 8
	case kind == reflect.Array && isByte(v.Type().Elem()):
		return v.Len()
	case kind == reflect.Slice && isByte(v.Type().Elem()):
		return 4
	case kind == reflect.String:
		return 4
	case kind == reflect.Array:
		if isFixedType(v.Type()) {
			return v.Len() * getTypeSize(v.Index(0))
		}
		return 4
	case kind == reflect.Struct:
		if isFixedType(v.Type()) {
			var size int
			for i := 0; i < v.NumField(); i++ {
				size += getTypeSize(v.Field(i))
			}
			return size
		}
		return 4
	case kind == reflect.Slice:
		return 4
	default:
		return 0
	}
}

type encoderFunc func(e *encodeState, v reflect.Value) error

var encoderCache sync.Map // map[reflect.Type]encoderFunc

func typeEncoder(t reflect.Type) (encoderFunc, error) {
	if fi, ok := encoderCache.Load(t); ok {
		return fi.(encoderFunc), nil
	}
	f, err := newTypeEncoder(t)
	if err != nil {
		return nil, err
	}
	encoderCache.Store(t, f)
	return f, nil
}

var (
	encoderInterface = reflect.TypeOf(new(Encoder)).Elem()
	decoderInterface = reflect.TypeOf(new(Decoder)).Elem()
	bigInt           = reflect.TypeOf(big.Int{})
	big0             = big.NewInt(0)
)

func newTypeEncoder(t reflect.Type) (encoderFunc, error) {
	kind := t.Kind()
	switch {
	case t.Implements(encoderInterface):
		return encodeEncoder, nil
	case kind != reflect.Ptr && reflect.PtrTo(t).Implements(encoderInterface):
		return encodeEncoderNoPtr, nil
	case kind == reflect.Interface:
		return encodeInterface, nil
	case kind == reflect.Ptr:
		return encodePtr, nil
	case t.AssignableTo(reflect.PtrTo(bigInt)):
		return encodeBigIntPtr, nil
	case t.AssignableTo(bigInt):
		return encodeBigIntNoPtr, nil
	case t.Kind() == reflect.Bool:
		return encodeBool, nil
	case isUint(t):
		return encodeUint, nil
	case isByte(t):
		return encodeUint, nil
	case kind == reflect.Array && isByte(t.Elem()):
		return encodeByteArray, nil
	case kind == reflect.Slice && isByte(t.Elem()):
		return encodeByteSlice, nil
	case kind == reflect.String:
		return encodeString, nil
	case kind == reflect.Slice:
		if t.Elem().Kind() == reflect.Interface {
			return nil, errors.New("ssz: interface slice is not SSZ-serializable")
		}
		return encodeSlice, nil
	case kind == reflect.Array:
		return encodeArray, nil
	case kind == reflect.Struct:
		return encodeStruct, nil
	default:
		return nil, fmt.Errorf("ssz: type %v is not SSZ-serializable", kind)
	}
}

type DecoderFunc func(s *Stream, v reflect.Value) error

var decoderCache sync.Map // map[reflect.Type]encoderFunc

func typeDecoder(t reflect.Type) (DecoderFunc, error) {
	if fi, ok := decoderCache.Load(t); ok {
		return fi.(DecoderFunc), nil
	}
	f, err := newTypeDecoder(t)
	if err != nil {
		return nil, err
	}
	decoderCache.Store(t, f)
	return f, nil
}

func newTypeDecoder(t reflect.Type) (DecoderFunc, error) {
	kind := t.Kind()
	switch {
	case t.Implements(decoderInterface):
		return decodeDecoder, nil
	case kind != reflect.Ptr && reflect.PtrTo(t).Implements(decoderInterface):
		return decodeDecoderNoPtr, nil
	//case kind == reflect.Interface:
	//	return encodeInterface, nil
	//case kind == reflect.Ptr:
	//	return encodePtr, nil
	//case t.AssignableTo(reflect.PtrTo(bigInt)):
	//	return encodeBigIntPtr, nil
	//case t.AssignableTo(bigInt):
	//	return encodeBigIntNoPtr, nil
	case t.Kind() == reflect.Bool:
		return decodeBool, nil
	case isUint(t):
		return decodeUint, nil
	//case kind == reflect.Array && isByte(t.Elem()):
	//	return encodeByteArray, nil
	//case kind == reflect.Slice && isByte(t.Elem()):
	//	return encodeByteSlice, nil
	//case kind == reflect.String:
	//	return encodeString, nil
	case kind == reflect.Slice:
		if t.Elem().Kind() == reflect.Interface {
			return nil, errors.New("ssz: interface slice is not SSZ-serializable")
		}
		return decodeSlice, nil
	case kind == reflect.Array:
		return decodeArray, nil
	//case kind == reflect.Struct:
	//	return encodeStruct, nil
	default:
		return nil, fmt.Errorf("ssz: type %v is not SSZ-serializable", kind)
	}
}
