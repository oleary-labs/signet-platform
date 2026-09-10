package chain

import (
	"encoding/hex"
	"strings"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		t.Fatalf("fixture is not hex: %v", err)
	}
	return b
}

// Real return data from the alpha group on mainnet, 0x86fE2814…, which has one
// SIWE domain. Hand-built fixtures agree with themselves; this one agrees with
// a deployed contract.
func TestSiweDomainsDecodesMainnetReturnData(t *testing.T) {
	raw := mustHex(t, "0x"+
		"0000000000000000000000000000000000000000000000000000000000000020"+ // offset to array
		"0000000000000000000000000000000000000000000000000000000000000001"+ // length 1
		"0000000000000000000000000000000000000000000000000000000000000020"+ // offset to string
		"000000000000000000000000000000000000000000000000000000000000000d"+ // length 13
		"6170702e73666c75762e6f726700000000000000000000000000000000000000") // "app.sfluv.org"

	d := decoder{raw}
	base, err := d.offsetAt(0)
	if err != nil {
		t.Fatalf("offset: %v", err)
	}
	got, err := d.stringArrayAt(base)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0] != "app.sfluv.org" {
		t.Fatalf("got %q, want [app.sfluv.org]", got)
	}
}

// An unbound resolver returns three zero words. The zero address is the
// contract's sentinel for "not configured", so the caller must be able to tell
// that apart from a resolver at address zero — which is why GroupState leaves
// the field nil rather than storing a zeroed binding.
func TestAuthResolverDecodesTheUnboundSentinel(t *testing.T) {
	raw := mustHex(t, "0x"+strings.Repeat("00", 96))

	got, err := decodeAuthResolver(decoder{raw}, 0)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Resolver != zeroAddress {
		t.Errorf("resolver = %q, want the zero address", got.Resolver)
	}
	if got.ChainID != 0 || got.RequireCanonicalSubject {
		t.Errorf("unbound binding should be zero throughout: %+v", got)
	}
}

func TestAuthResolverDecodesABoundResolver(t *testing.T) {
	raw := mustHex(t, "0x"+
		"000000000000000000000000000000000000000000000000000000000000a4ec"+ // chainId 42220 (Celo)
		"000000000000000000000000431d7455353c7d31e762d468c5818d5843ebc5a5"+ // resolver
		"0000000000000000000000000000000000000000000000000000000000000001") // requireCanonicalSubject

	got, err := decodeAuthResolver(decoder{raw}, 0)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ChainID != 42220 {
		t.Errorf("chainId = %d, want 42220", got.ChainID)
	}
	if !strings.EqualFold(got.Resolver, "0x431d7455353c7d31e762d468c5818d5843ebc5a5") {
		t.Errorf("resolver = %q", got.Resolver)
	}
	if !got.RequireCanonicalSubject {
		t.Error("requireCanonicalSubject should be true")
	}
}
