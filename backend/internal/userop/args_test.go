package userop

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// pad left-pads a value to a 32-byte ABI word.
func pad(b []byte) []byte {
	w := make([]byte, 32)
	copy(w[32-len(b):], b)
	return w
}

func decodedWith(inner []byte) *Decoded { return &Decoded{Inner: inner} }

// removeIssuer(bytes32) — the shape the issuer route checks.
func TestWordReadsStaticArgument(t *testing.T) {
	want, _ := hex.DecodeString(
		"1111111111111111111111111111111111111111111111111111111111111111")
	inner := append([]byte{0x1c, 0xc5, 0x54, 0xf2}, want...)

	got, ok := decodedWith(inner).Word(0)
	if !ok {
		t.Fatal("Word(0) reported the argument as missing")
	}
	if !bytes.Equal(got[:], want) {
		t.Fatalf("Word(0) = %x, want %x", got, want)
	}
}

// A call that names a method but carries no argument must not read as a call
// on the zero hash — that would let removeIssuer(0x00..) pass a check meant to
// confirm which issuer is being removed.
func TestWordRefusesTruncatedArgument(t *testing.T) {
	for _, tc := range []struct {
		name  string
		inner []byte
	}{
		{"selector only", []byte{0x1c, 0xc5, 0x54, 0xf2}},
		{"half a word", append([]byte{0x1c, 0xc5, 0x54, 0xf2}, make([]byte, 16)...)},
		{"empty", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := decodedWith(tc.inner).Word(0); ok {
				t.Fatal("Word(0) accepted calldata too short to contain it")
			}
		})
	}
}

func TestWordRefusesNegativeIndex(t *testing.T) {
	inner := append([]byte{0x1c, 0xc5, 0x54, 0xf2}, make([]byte, 32)...)
	if _, ok := decodedWith(inner).Word(-1); ok {
		t.Fatal("Word(-1) was accepted")
	}
}

// addAuthKey(bytes) — the shape the credential route checks.
func TestBytesArgReadsDynamicArgument(t *testing.T) {
	key := make([]byte, 34)
	for i := range key {
		key[i] = byte(i)
	}
	inner := []byte{0x6d, 0x6a, 0x24, 0x1f}
	inner = append(inner, pad([]byte{0x20})...) // offset
	inner = append(inner, pad([]byte{34})...)   // length
	inner = append(inner, key...)
	inner = append(inner, make([]byte, 30)...) // right padding to a word

	got, ok := decodedWith(inner).BytesArg(0)
	if !ok {
		t.Fatal("BytesArg(0) reported the argument as missing")
	}
	if !bytes.Equal(got, key) {
		t.Fatalf("BytesArg(0) = %x, want %x", got, key)
	}
}

// A length or offset pointing outside the calldata must be refused rather than
// panicking or returning a truncated key that could compare equal to nothing.
func TestBytesArgRefusesOutOfBounds(t *testing.T) {
	base := []byte{0x6d, 0x6a, 0x24, 0x1f}

	cases := map[string][]byte{
		"offset past end": append(append(base, pad([]byte{0xff})...), pad([]byte{1})...),
		"length past end": append(append(base, pad([]byte{0x20})...), pad([]byte{0xff})...),
		"no length word":  append(base, pad([]byte{0x20})...),
		"huge offset": append(append(base, bytes.Repeat([]byte{0xff}, 32)...),
			pad([]byte{1})...),
		"huge length": append(append(base, pad([]byte{0x20})...),
			bytes.Repeat([]byte{0xff}, 32)...),
	}
	for name, inner := range cases {
		t.Run(name, func(t *testing.T) {
			if got, ok := decodedWith(inner).BytesArg(0); ok {
				t.Fatalf("BytesArg(0) accepted out-of-bounds encoding, returned %x", got)
			}
		})
	}
}
