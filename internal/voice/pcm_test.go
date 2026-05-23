package voice

import (
	"bytes"
	"testing"
)

func TestInt16ToLEBytes(t *testing.T) {
	tests := []struct {
		name string
		in   []int16
		want []byte
	}{
		{
			name: "empty",
			in:   nil,
			want: []byte{},
		},
		{
			name: "single positive",
			in:   []int16{0x0102},
			want: []byte{0x02, 0x01}, // little-endian
		},
		{
			name: "zero and max",
			in:   []int16{0, 32767},
			want: []byte{0x00, 0x00, 0xff, 0x7f},
		},
		{
			name: "negative (two's complement)",
			in:   []int16{-1, -32768},
			want: []byte{0xff, 0xff, 0x00, 0x80},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := int16ToLEBytes(tc.in)
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("int16ToLEBytes(%v) = % x, want % x", tc.in, got, tc.want)
			}
			if len(got) != 2*len(tc.in) {
				t.Fatalf("len = %d, want %d (2 bytes per sample)", len(got), 2*len(tc.in))
			}
		})
	}
}
