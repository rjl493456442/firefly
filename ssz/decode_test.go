package ssz

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

func TestDecode(t *testing.T) {
	var cases = []struct {
		input    interface{}
		expected interface{}
		err      error
	}{
		{true, new(bool), nil},
		{false, new(bool), nil},
		{uint8(0), new(uint8), nil},
		{uint8(255), new(uint8), nil},
		{uint16(0), new(uint16), nil},
		{uint16(65535), new(uint16), nil},
		{uint32(0), new(uint32), nil},
		{uint32(4294967295), new(uint32), nil},
		{uint64(0), new(uint64), nil},
		{uint64(18446744073709551615), new(uint64), nil},
		{[]bool{true, false}, new([]bool), nil},
		{[2]bool{true, false}, new([2]bool), nil},
	}
	var buffer bytes.Buffer
	for i, c := range cases {
		err := Encode(&buffer, c.input)
		if err != nil {
			t.Fatalf("case:%d encode failed, err %v", i, err)
		}
		err = Decode(&buffer, c.expected)
		if !reflect.DeepEqual(err, c.err) {
			t.Fatalf("case:%d decode error mismatch, want %v, have %v", i, c.err, err)
		}
		fmt.Printf("%v\n", reflect.ValueOf(c.expected).Elem())
		buffer.Reset()
	}
}
