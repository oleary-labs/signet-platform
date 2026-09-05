// Package chain reads Signet's on-chain state over JSON-RPC.
//
// It carries a small, purpose-built ABI codec rather than a general one. The
// platform only ever performs `eth_call` against two known contracts, so the
// codec supports exactly the types those functions use — address, uint256,
// bool, bytes, string, and arrays/tuples of them — and refuses anything else
// rather than guessing.
package chain

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/sha3"
)

const wordSize = 32

// selector returns the 4-byte function selector for a canonical signature such
// as "getNode(address)".
func selector(signature string) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(signature))
	return h.Sum(nil)[:4]
}

// encodeCall builds calldata for a function whose arguments are all static
// 32-byte words (which covers every call the platform makes).
func encodeCall(signature string, words ...[]byte) []byte {
	out := selector(signature)
	for _, w := range words {
		padded := make([]byte, wordSize)
		copy(padded[wordSize-len(w):], w)
		out = append(out, padded...)
	}
	return out
}

// addressWord left-pads a 20-byte address into an ABI word.
func addressWord(addr string) ([]byte, error) {
	raw, err := decodeHex(addr)
	if err != nil {
		return nil, err
	}
	if len(raw) != 20 {
		return nil, fmt.Errorf("address must be 20 bytes, got %d", len(raw))
	}
	word := make([]byte, wordSize)
	copy(word[12:], raw)
	return word, nil
}

// decoder reads ABI-encoded return data. Every accessor is bounds-checked, so
// truncated or malicious return data produces an error rather than a panic in
// the middle of a request.
type decoder struct{ data []byte }

func (d decoder) word(offset int) ([]byte, error) {
	if offset < 0 || offset+wordSize > len(d.data) {
		return nil, fmt.Errorf("abi: read at %d exceeds %d bytes of return data", offset, len(d.data))
	}
	return d.data[offset : offset+wordSize], nil
}

// uintAt reads a uint256 as a big.Int.
func (d decoder) uintAt(offset int) (*big.Int, error) {
	w, err := d.word(offset)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(w), nil
}

// intAt reads a uint256 that is expected to fit in an int64 (counts,
// timestamps, thresholds). A value that does not fit is an error, not a
// silent wrap.
func (d decoder) intAt(offset int) (int64, error) {
	n, err := d.uintAt(offset)
	if err != nil {
		return 0, err
	}
	if !n.IsInt64() {
		return 0, fmt.Errorf("abi: value at %d does not fit in int64", offset)
	}
	return n.Int64(), nil
}

// offsetAt reads a dynamic-type head word as a byte offset.
func (d decoder) offsetAt(offset int) (int, error) {
	n, err := d.intAt(offset)
	if err != nil {
		return 0, err
	}
	if n < 0 || n > int64(len(d.data)) {
		return 0, fmt.Errorf("abi: offset %d is outside the return data", n)
	}
	return int(n), nil
}

func (d decoder) boolAt(offset int) (bool, error) {
	w, err := d.word(offset)
	if err != nil {
		return false, err
	}
	for _, b := range w[:31] {
		if b != 0 {
			return false, fmt.Errorf("abi: bool at %d has dirty high bytes", offset)
		}
	}
	return w[31] == 1, nil
}

func (d decoder) addressAt(offset int) (string, error) {
	w, err := d.word(offset)
	if err != nil {
		return "", err
	}
	return "0x" + encodeHex(w[12:]), nil
}

// bytesAt reads a dynamic `bytes` (or `string`) whose length word starts at
// base.
func (d decoder) bytesAt(base int) ([]byte, error) {
	n, err := d.intAt(base)
	if err != nil {
		return nil, err
	}
	start := base + wordSize
	if n < 0 || start+int(n) > len(d.data) {
		return nil, fmt.Errorf("abi: bytes at %d claims %d bytes past the end", base, n)
	}
	return d.data[start : start+int(n)], nil
}

func (d decoder) stringAt(base int) (string, error) {
	raw, err := d.bytesAt(base)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// addressArray decodes `address[]` returned as the sole (dynamic) return
// value.
func (d decoder) addressArray() ([]string, error) {
	base, err := d.offsetAt(0)
	if err != nil {
		return nil, err
	}
	n, err := d.intAt(base)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, n)
	for i := int64(0); i < n; i++ {
		addr, err := d.addressAt(base + wordSize + int(i)*wordSize)
		if err != nil {
			return nil, err
		}
		out = append(out, addr)
	}
	return out, nil
}

// bytesArray decodes `bytes[]` returned as the sole return value — the shape
// of SignetGroup.getAuthKeys().
func (d decoder) bytesArray() ([][]byte, error) {
	base, err := d.offsetAt(0)
	if err != nil {
		return nil, err
	}
	n, err := d.intAt(base)
	if err != nil {
		return nil, err
	}
	items := base + wordSize
	out := make([][]byte, 0, n)
	for i := int64(0); i < n; i++ {
		// Element offsets in a dynamic array are relative to the first element.
		rel, err := d.offsetAt(items + int(i)*wordSize)
		if err != nil {
			return nil, err
		}
		raw, err := d.bytesAt(items + rel)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}

// stringArrayAt decodes a `string[]` whose length word starts at base.
func (d decoder) stringArrayAt(base int) ([]string, error) {
	n, err := d.intAt(base)
	if err != nil {
		return nil, err
	}
	items := base + wordSize
	out := make([]string, 0, n)
	for i := int64(0); i < n; i++ {
		rel, err := d.offsetAt(items + int(i)*wordSize)
		if err != nil {
			return nil, err
		}
		s, err := d.stringAt(items + rel)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func decodeHex(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(s)), "0x")
	if len(s)%2 == 1 {
		s = "0" + s
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		hi, err := hexNibble(s[i*2])
		if err != nil {
			return nil, err
		}
		lo, err := hexNibble(s[i*2+1])
		if err != nil {
			return nil, err
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func hexNibble(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	}
	return 0, fmt.Errorf("abi: %q is not a hex digit", string(c))
}

const hexDigits = "0123456789abcdef"

func encodeHex(b []byte) string {
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexDigits[v>>4]
		out[i*2+1] = hexDigits[v&0x0f]
	}
	return string(out)
}

// uint64Word encodes a uint64 into an ABI word, used where a call takes a
// numeric argument.
func uint64Word(v uint64) []byte {
	word := make([]byte, wordSize)
	binary.BigEndian.PutUint64(word[24:], v)
	return word
}
