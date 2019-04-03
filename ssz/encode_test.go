package ssz

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"testing"
)

func TestEncode(t *testing.T) {
	var cases = []struct {
		input    interface{}
		expected []byte
		err      error
	}{
		{false, []byte{0x00}, nil},
		{true, []byte{0x01}, nil},
		{uint8(0), []byte{0x00}, nil},
		{uint8(1), []byte{0x01}, nil},
		{uint8(255), []byte{0xff}, nil},
		{uint16(0), []byte{0x00, 0x00}, nil},
		{uint16(256), []byte{0x00, 0x01}, nil},
		{uint16(65535), []byte{0xff, 0xff}, nil},
		{uint32(0), []byte{0x00, 0x00, 0x00, 0x00}, nil},
		{uint32(65536), []byte{0x00, 0x00, 0x01, 0x00}, nil},
		{uint32(4294967295), []byte{0xff, 0xff, 0xff, 0xff}, nil},
		{uint64(0), []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, nil},
		{uint64(4294967296), []byte{0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}, nil},
		{uint64(18446744073709551615), []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, nil},
		{int8(0), nil, errors.New("ssz: type int8 is not SSZ-serializable")},
		{int16(0), nil, errors.New("ssz: type int16 is not SSZ-serializable")},
		{int32(0), nil, errors.New("ssz: type int32 is not SSZ-serializable")},
		{int64(0), nil, errors.New("ssz: type int64 is not SSZ-serializable")},
		{new(big.Int).SetBytes([]byte{0x00}), nil, errors.New("ssz: only 9-32 bytes *big.Int are supported")},
		{new(big.Int).SetInt64(-1), nil, errors.New("ssz: cannot encode negative *big.Int")},
		{new(big.Int).SetBytes([]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}), []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, nil},
		{[2]byte{0x01, 0x10}, []byte{0x01, 0x10}, nil},
		{[]byte{0x01, 0x10}, []byte{0x01, 0x10}, nil},
		{"hello-world", []byte{104, 101, 108, 108, 111, 45, 119, 111, 114, 108, 100}, nil},
		{[2]bool{true, false}, []byte{0x01, 0x00}, nil},
		{[]bool{true, false}, []byte{0x01, 0x00}, nil},
		{[2]uint16{1, 0}, []byte{0x01, 0x00, 0x00, 0x00}, nil},
		{[]uint16{1, 0}, []byte{0x01, 0x00, 0x00, 0x00}, nil},
		{[][]byte{{0xfe, 0xff}, {0x01, 0x02}}, []byte{0x08, 0x00, 0x00, 0x00, 0x0a, 0x00, 0x00, 0x00, 0xfe, 0xff, 0x01, 0x02}, nil},
		{struct {
			A bool
			B uint8
			C []byte
		}{false, uint8(255), []byte{0xff}}, []byte{0x00, 0xff, 0x06, 0x00, 0x00, 0x00, 0xff}, nil},
	}
	var buffer bytes.Buffer
	for i, c := range cases {
		err := Encode(&buffer, c.input)
		if !reflect.DeepEqual(err, c.err) {
			t.Fatalf("case:%d encode error mismatch, want %v, have %v", i, c.err, err)
		}
		if bytes.Compare(buffer.Bytes(), c.expected) != 0 {
			t.Fatalf("case:%d encode result mismatch, want %v, have %v", i, c.expected, buffer.Bytes())
		}
		buffer.Reset()
	}

	var a = [3]interface{}{[]byte{0x02}, true, 199}
	fmt.Println(isFixedType(reflect.TypeOf(a)))
}
