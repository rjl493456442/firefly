package ssz

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"reflect"
	"sync"
)

var (
	errNoPointer     = errors.New("ssz: interface given to Decode must be a pointer")
	errDecodeIntoNil = errors.New("ssz: pointer given to Decode must not be nil")

	streamPool = sync.Pool{
		New: func() interface{} { return new(Stream) },
	}
)

// Decoder is implemented by types that require custom RLP
// decoding rules or need to decode into private fields.
//
// The DecodeRLP method should read one value from the given
// Stream. It is not forbidden to read less or more, but it might
// be confusing.
type Decoder interface {
	DecodeSSZ(*Stream) error
}

func Decode(r io.Reader, val interface{}) error {
	stream := streamPool.Get().(*Stream)
	defer streamPool.Put(stream)

	stream.Reset(r)
	return stream.Decode(val)
}

// This decoder is used for non-pointer values of types
// that implement the Decoder interface using a pointer receiver.
func decodeDecoderNoPtr(s *Stream, val reflect.Value) error {
	return val.Addr().Interface().(Decoder).DecodeSSZ(s)
}

func decodeDecoder(s *Stream, val reflect.Value) error {
	// Decoder instances are not handled using the pointer rule if the type
	// implements Decoder with pointer receiver (i.e. always)
	// because it might handle empty values specially.
	// We need to allocate one here in this case, like makePtrDecoder does.
	if val.Kind() == reflect.Ptr && val.IsNil() {
		val.Set(reflect.New(val.Type().Elem()))
	}
	return val.Interface().(Decoder).DecodeSSZ(s)
}

func decodeBool(s *Stream, val reflect.Value) error {
	b, err := s.readByte()
	if err != nil {
		return err
	}
	res := false
	if b == 0x01 {
		res = true
	}
	val.SetBool(res)
	return nil
}

func decodeUint(s *Stream, val reflect.Value) error {
	size := getTypeSize(val)
	if err := s.readBytes(s.scratch[:size]); err != nil {
		return err
	}
	switch size {
	case 1:
		val.SetUint(uint64(int8(s.scratch[0])))
	case 2:
		val.SetUint(uint64(binary.LittleEndian.Uint16(s.scratch[:2])))
	case 4:
		val.SetUint(uint64(binary.LittleEndian.Uint32(s.scratch[:4])))
	case 8:
		val.SetUint(binary.LittleEndian.Uint64(s.scratch[:8]))
	}
	return nil
}

func decodeSlice(s *Stream, val reflect.Value) error {
	df, err := typeDecoder(val.Type().Elem())
	if err != nil {
		return nil
	}
	if !isFixedType(val.Type().Elem()) {
		offset, err := s.readOffset()
		if err != nil {
			return err
		}
		ss, err := s.newSectionStream(int64(offset), int(s.size())-int(offset))
		if err != nil {
			return err
		}
		return decodeSliceElems(ss, val, df)
	}
	return decodeSliceElems(s, val, df)
}

func decodeArray(s *Stream, val reflect.Value) error {
	var i int
	if val.Kind() == reflect.Interface {
		for i := 0; i < val.Len(); i++ {
			df, err := typeDecoder(val.Index(i).Type())
			if err != nil {
				return nil
			}
			if isFixedType(val.Index(i).Type()) {
				df(s, val.Index(i))
			} else {
				offset, err := s.readOffset()
				if err != nil {
					return err
				}
				ss, err := s.newSectionStream(int64(offset), int(s.size())-int(offset))
				if err != nil {
					return err
				}
				df(ss, val.Index(i))
			}
		}
	} else {
		df, err := typeDecoder(val.Type().Elem())
		if err != nil {
			return nil
		}
		for ; i < val.Len(); i++ {
			if err := df(s, val.Index(i)); err == io.EOF {
				err = nil
				break
			} else if err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeSliceElems(s *Stream, val reflect.Value, elemdec DecoderFunc) error {
	i := 0
	for ; ; i++ {
		// grow slice if necessary
		if i >= val.Cap() {
			newcap := val.Cap() + val.Cap()/2
			if newcap < 4 {
				newcap = 4
			}
			newv := reflect.MakeSlice(val.Type(), val.Len(), newcap)
			reflect.Copy(newv, val)
			val.Set(newv)
		}
		if i >= val.Len() {
			val.SetLen(i + 1)
		}
		// decode into element
		if err := elemdec(s, val.Index(i)); err == io.EOF {
			err = nil
			break
		} else if err != nil {
			return err
		}
	}
	if i < val.Len() {
		val.SetLen(i)
	}
	return nil
}

// ByteReader must be implemented by any input reader for a Stream. It
// is implemented by e.g. bufio.Reader and bytes.Reader.
type Reader interface {
	io.Reader
	io.ByteReader
}

type Stream struct {
	r *bytes.Reader

	// scratch is used for caching small size value temporarily instead of allocating
	// every time.
	scratch [32]byte
}

func NewStream(r io.Reader) (*Stream, error) {
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &Stream{r: bytes.NewReader(buf)}, nil
}

func (s *Stream) Reset(r io.Reader) error {
	if br, ok := r.(*bytes.Reader); ok {
		s.r = br
		return nil
	}
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	s.r = bytes.NewReader(buf)
	return nil
}

func (s *Stream) Decode(val interface{}) error {
	if val == nil {
		return errDecodeIntoNil
	}
	rval := reflect.ValueOf(val)
	rtyp := rval.Type()
	if rtyp.Kind() != reflect.Ptr {
		return errNoPointer
	}
	if rval.IsNil() {
		return errDecodeIntoNil
	}
	df, err := newTypeDecoder(rtyp.Elem())
	if err != nil {
		return err
	}
	return df(s, rval.Elem())
}

func (s *Stream) newSectionStream(offset int64, length int) (*Stream, error) {
	buf := make([]byte, length)
	if _, err := s.r.ReadAt(buf, offset); err != nil {
		return nil, err
	}
	return NewStream(bytes.NewReader(buf))
}

func (s *Stream) size() int64 {
	return s.r.Size()
}

func (s *Stream) readBytes(p []byte) (err error) {
	var nn, n int
	for n < len(p) && err == nil {
		nn, err = s.r.Read(p[n:])
		n += nn
	}
	if err == io.EOF && n < len(p) {
		err = io.ErrUnexpectedEOF
	}
	return err
}

func (s *Stream) readByte() (byte, error) {
	return s.r.ReadByte()
}

func (s *Stream) readOffset() (uint32, error) {
	if err := s.readBytes(s.scratch[:4]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(s.scratch[:4]), nil
}
