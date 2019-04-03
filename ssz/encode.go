package ssz

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"sync"
)

var (
	boolFalse = []byte{0x00}
	boolTrue  = []byte{0x01}
)

// Encoder is implemented by types that require custom encoding rules
// or want to encode private fields.
type Encoder interface {
	// EncodeSSZ should write the SSZ encoding of its receiver to w.
	// If the implementation is a pointer method, it may also be
	// called for nil pointers.
	EncodeSSZ(io.Writer) error
}

func Encode(w io.Writer, val interface{}) error {
	es := encodeStatePool.Get().(*encodeState)
	defer encodeStatePool.Put(es)
	es.reset()

	if err := es.encode(val); err != nil {
		return err
	}
	return es.toWriter(w)
}

// encodeState are pooled.
var encodeStatePool = sync.Pool{
	New: func() interface{} { return &encodeState{} },
}

type encodeState struct {
	// buffer is the accumulated output of the serialized representation.
	buffer bytes.Buffer

	// auxBuffer is the accumulated output of the serialized representations
	// of only the variable-size types.
	auxBuffer bytes.Buffer

	// scratch is used for caching small size value temporarily instead of allocating
	// every time.
	scratch [32]byte
}

// Write writes len(p) bytes from p to the underlying accumulated buffer.
func (es *encodeState) Write(p []byte) (n int, err error) {
	return es.buffer.Write(p)
}

// toWriter writes accumulated output in buffer to given writer.
func (es *encodeState) toWriter(w io.Writer) error {
	_, err := es.buffer.WriteTo(w)
	return err
}

// reset resets the buffers to be empty.
func (es *encodeState) reset() {
	es.buffer.Reset()
	es.auxBuffer.Reset()
}

func (es *encodeState) encode(val interface{}) error {
	rval := reflect.ValueOf(val)
	ef, err := typeEncoder(rval.Type())
	if err != nil {
		return err
	}
	return ef(es, rval)
}

// encodeEncoder handles pointer values that implement Encoder.
func encodeEncoder(e *encodeState, v reflect.Value) error {
	return v.Interface().(Encoder).EncodeSSZ(e)
}

// encodeEncoderNoPtr handles non-pointer values that implement Encoder
// with a pointer receiver.
func encodeEncoderNoPtr(e *encodeState, v reflect.Value) error {
	if !v.CanAddr() {
		// We can't get the address. It would be possible to make the
		// value addressable by creating a shallow copy, but this
		// creates other problems so we're not doing it (yet).
		//
		// package json simply doesn't call MarshalJSON for cases like
		// this, but encodes the value as if it didn't implement the
		// interface. We don't want to handle it that way.
		return fmt.Errorf("ssz: game over: unadressable value of type %v, EncodeSSZ is pointer method", v.Type())
	}
	return v.Addr().Interface().(Encoder).EncodeSSZ(e)
}

func encodeInterface(e *encodeState, v reflect.Value) error {
	if v.IsNil() {
		return errors.New("ssz: empty interface is not supported by ssz")
	}
	ef, err := typeEncoder(v.Elem().Type())
	if err != nil {
		return err
	}
	return ef(e, v.Elem())
}

func encodePtr(e *encodeState, v reflect.Value) error {
	if v.IsNil() {
		return errors.New("ssz: empty pointer is not supported by ssz")
	}
	ef, err := typeEncoder(v.Elem().Type())
	if err != nil {
		return err
	}
	return ef(e, v.Elem())
}

func encodeBigIntPtr(e *encodeState, v reflect.Value) error {
	ptr := v.Interface().(*big.Int)
	if ptr == nil {
		return errors.New("ssz: empty *big.Int is not supported by ssz")
	}
	return encodeBigInt(e, ptr)
}

func encodeBigIntNoPtr(e *encodeState, v reflect.Value) error {
	i := v.Interface().(big.Int)
	return encodeBigInt(e, &i)
}

func encodeBigInt(e *encodeState, i *big.Int) error {
	if cmp := i.Cmp(big0); cmp == -1 {
		return errors.New("ssz: cannot encode negative *big.Int")
	}
	// SSZ only support uint128, uint256
	bigEndian := i.Bytes()
	if len(bigEndian) < 9 || len(bigEndian) > 32 {
		return errors.New("ssz: only 9-32 bytes *big.Int are supported")
	}
	for i, b := range bigEndian {
		e.scratch[len(bigEndian)-i-1] = b
	}
	length := 16
	if len(bigEndian) >= 17 {
		length = 32
	}
	for i := len(bigEndian); i < length; i++ {
		e.scratch[i] = 0x00
	}
	e.buffer.Write(e.scratch[:length])
	return nil
}

// encodeLength writes variable's length into accumulated buffer
// in little-endian format.
func encodeLength(e *encodeState, len uint32) {
	binary.LittleEndian.PutUint32(e.scratch[:4], uint32(len))
	e.buffer.Write(e.scratch[:4])
}

func encodeBool(e *encodeState, v reflect.Value) error {
	val := boolFalse
	if v.Bool() {
		val = boolTrue
	}
	e.buffer.Write(val)
	return nil
}

func encodeUint(e *encodeState, v reflect.Value) error {
	val := v.Uint()
	switch v.Kind() {
	case reflect.Uint8:
		e.scratch[0] = byte(val)
		e.buffer.Write(e.scratch[:1])
	case reflect.Uint16:
		binary.LittleEndian.PutUint16(e.scratch[:2], uint16(val))
		e.buffer.Write(e.scratch[:2])
	case reflect.Uint32:
		binary.LittleEndian.PutUint32(e.scratch[:4], uint32(val))
		e.buffer.Write(e.scratch[:4])
	case reflect.Uint64:
		binary.LittleEndian.PutUint64(e.scratch[:8], val)
		e.buffer.Write(e.scratch[:8])
	}
	return nil
}

func encodeByteArray(e *encodeState, v reflect.Value) error {
	if !v.CanAddr() {
		// Slice requires the value to be addressable.
		// Make it addressable by copying.
		copy := reflect.New(v.Type()).Elem()
		copy.Set(v)
		v = copy
	}
	size := v.Len()
	slice := v.Slice(0, size).Bytes()
	e.buffer.Write(slice)
	return nil
}

func encodeByteSlice(e *encodeState, v reflect.Value) error {
	e.buffer.Write(v.Bytes())
	return nil
}

func encodeString(e *encodeState, v reflect.Value) error {
	e.buffer.Write([]byte(v.String()))
	return nil
}

func encodeArray(e *encodeState, v reflect.Value) error {
	var (
		s1size        int
		heterogeneous bool
	)
	if v.Type().Elem().Kind() == reflect.Interface {
		for i := 0; i < v.Len(); i++ {
			s1size += getTypeSize(v.Index(i))
		}
		heterogeneous = true
	} else if !isFixedType(v.Type()) {
		s1size = 4 * v.Len()
	}

	var (
		ef  encoderFunc
		err error
	)
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if ef == nil || heterogeneous {
			ef, err = typeEncoder(elem.Type())
			if err != nil {
				return nil
			}
		}
		if isFixedType(elem.Type()) {
			if err := ef(e, elem); err != nil {
				return err
			}
		} else {
			encodeLength(e, uint32(s1size+e.auxBuffer.Len()))

			inner := encodeStatePool.Get().(*encodeState)
			inner.reset()

			ef(inner, elem)
			e.auxBuffer.Write(inner.buffer.Bytes())
			encodeStatePool.Put(inner)
		}
	}
	e.buffer.Write(e.auxBuffer.Bytes())
	e.auxBuffer.Reset()
	return nil
}

func encodeSlice(e *encodeState, v reflect.Value) error {
	if v.IsNil() {
		// Write empty slice
		return nil
	}
	var (
		s1size    int
		elemFixed bool
	)
	elemFixed = isFixedType(v.Type().Elem())
	if !elemFixed {
		s1size = v.Len() * 4
	}
	ef, err := typeEncoder(v.Type().Elem())
	if err != nil {
		return nil
	}
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if elemFixed {
			ef(e, elem)
		} else {
			encodeLength(e, uint32(s1size+e.auxBuffer.Len()))

			inner := encodeStatePool.Get().(*encodeState)
			inner.reset()

			ef(inner, elem)
			e.auxBuffer.Write(inner.buffer.Bytes())
			encodeStatePool.Put(inner)
		}
	}
	e.buffer.Write(e.auxBuffer.Bytes())
	e.auxBuffer.Reset()
	return nil
}

func walkStruct(v reflect.Value, cb func(int, reflect.Value) error) error {
	typ := v.Type()
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.PkgPath == "" { // exported
			// todo(rjl493456442) support ssz tag
			if err := cb(i, v.Field(i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func encodeStruct(e *encodeState, v reflect.Value) error {
	var s1size int
	walkStruct(v, func(i int, value reflect.Value) error {
		s1size += getTypeSize(value)
		return nil
	})
	walkStruct(v, func(i int, value reflect.Value) error {
		ef, err := typeEncoder(value.Type())
		if err != nil {
			return err
		}
		if isFixedType(value.Type()) {
			ef(e, value)
		} else {
			binary.LittleEndian.PutUint32(e.scratch[:4], uint32(s1size+e.auxBuffer.Len()))
			e.buffer.Write(e.scratch[:4])

			inner := encodeStatePool.Get().(*encodeState)
			defer encodeStatePool.Put(inner)
			inner.reset()

			ef(inner, value)
			e.auxBuffer.Write(inner.buffer.Bytes())
		}
		return nil
	})
	e.buffer.Write(e.auxBuffer.Bytes())
	e.auxBuffer.Reset()
	return nil
}
