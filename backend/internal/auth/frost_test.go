package auth

import (
	"encoding/hex"
	"strings"
	"testing"
)

// Vector produced by signet-protocol's cmd/testvector against the real FROST
// implementation, and also consumed by FROSTVerifier.t.sol. If this test fails,
// the platform's login route disagrees with both the nodes and the on-chain
// verifier — not the other way round.
const (
	vecGroupPubKey = "03ba81688507e7e2e2f29c90aebe66cc05aef00ad25fb79a8f2989fa7aab81ba8f"
	vecMsgHash     = "4badeece6c056bd51ae542637718a0c9ae9ea5cf5c3c6b5687ca9a3b77319067"
	vecSigRx       = "b3480f95d8a8a830ddd23b41e758b8b27cb11d082ee0e466ababed3a357e1632"
	vecSigZ        = "0c2a0df0f0f991a6cbabe79d7d28c2178a9a704afbc718ca9a3766bef2825518"
	vecSigV        = 1
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		t.Fatalf("decode %q: %v", s, err)
	}
	return b
}

func vectorSignature(t *testing.T, v byte) []byte {
	t.Helper()
	sig := make([]byte, 65)
	copy(sig[0:32], mustHex(t, vecSigRx))
	copy(sig[32:64], mustHex(t, vecSigZ))
	sig[64] = v
	return sig
}

func TestVerifyFROSTAcceptsKnownVector(t *testing.T) {
	if err := VerifyFROST(mustHex(t, vecMsgHash), vectorSignature(t, vecSigV), mustHex(t, vecGroupPubKey)); err != nil {
		t.Fatalf("known-good FROST vector rejected: %v", err)
	}
}

func TestVerifyFROSTRejectsTampering(t *testing.T) {
	msg := mustHex(t, vecMsgHash)
	pub := mustHex(t, vecGroupPubKey)

	t.Run("flipped parity", func(t *testing.T) {
		if err := VerifyFROST(msg, vectorSignature(t, 0), pub); err == nil {
			t.Fatal("signature with the wrong R parity was accepted")
		}
	})

	t.Run("mutated message", func(t *testing.T) {
		bad := append([]byte(nil), msg...)
		bad[0] ^= 0x01
		if err := VerifyFROST(bad, vectorSignature(t, vecSigV), pub); err == nil {
			t.Fatal("signature verified against a different message")
		}
	})

	t.Run("mutated z", func(t *testing.T) {
		sig := vectorSignature(t, vecSigV)
		sig[63] ^= 0x01
		if err := VerifyFROST(msg, sig, pub); err == nil {
			t.Fatal("signature with a mutated z was accepted")
		}
	})

	t.Run("wrong group key", func(t *testing.T) {
		other := append([]byte(nil), pub...)
		other[32] ^= 0x01
		if err := VerifyFROST(msg, vectorSignature(t, vecSigV), other); err == nil {
			t.Fatal("signature verified under a different group key")
		}
	})

	t.Run("malformed lengths", func(t *testing.T) {
		if err := VerifyFROST(msg, vectorSignature(t, vecSigV)[:64], pub); err == nil {
			t.Fatal("64-byte signature was accepted")
		}
		if err := VerifyFROST(msg, vectorSignature(t, vecSigV), pub[:32]); err == nil {
			t.Fatal("32-byte group key was accepted")
		}
	})

	t.Run("zero z", func(t *testing.T) {
		sig := vectorSignature(t, vecSigV)
		for i := 32; i < 64; i++ {
			sig[i] = 0
		}
		if err := VerifyFROST(msg, sig, pub); err == nil {
			t.Fatal("zero scalar was accepted")
		}
	})
}

// expand_message_xmd test vectors from RFC 9380 Appendix K.1
// (expand_message_xmd SHA-256, DST "QUUX-V01-CS02-with-expander-SHA256-128").
func TestExpandMessageXMDMatchesRFC9380(t *testing.T) {
	dst := []byte("QUUX-V01-CS02-with-expander-SHA256-128")
	cases := []struct {
		msg  string
		size int
		want string
	}{
		{"", 32, "68a985b87eb6b46952128911f2a4412bbc302a9d759667f87f7a21d803f07235"},
		{"abc", 32, "d8ccab23b5985ccea865c6c97b6e5b8350e794e603b4b97902f53a8a0d605615"},
		{"abcdef0123456789", 32, "eff31487c770a893cfb36f912fbfcbff40d5661771ca4b2cb4eafe524333f5c1"},
		{"", 128, "af84c27ccfd45d41914fdff5df25293e221afc53d8ad2ac06d5e3e29485dadbee0d121587713a3e0dd4d5e69e93eb7cd4f5df4cd103e188cf60cb02edc3edf18eda8576c412b18ffb658e3dd6ec849469b979d444cf7b26911a08e63cf31f9dcc541708d3491184472c2c29bb749d4286b004ceb5ee6b9a7fa5b646c993f0ced"},
	}
	for _, tc := range cases {
		got, err := expandMessageXMD([]byte(tc.msg), dst, tc.size)
		if err != nil {
			t.Fatalf("expand(%q, %d): %v", tc.msg, tc.size, err)
		}
		if hex.EncodeToString(got) != tc.want {
			t.Errorf("expand(%q, %d) = %s, want %s", tc.msg, tc.size, hex.EncodeToString(got), tc.want)
		}
	}
}
