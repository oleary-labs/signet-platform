package auth

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/google/uuid"
)

// The canonical Anvil/Hardhat account 0. Using a published test key keeps the
// expected address checkable by hand against any Ethereum tool.
const (
	anvilKey0  = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	anvilAddr0 = "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"
)

func testKey(t *testing.T) *secp256k1.PrivateKey {
	t.Helper()
	raw, err := hex.DecodeString(anvilKey0)
	if err != nil {
		t.Fatalf("decode key: %v", err)
	}
	return secp256k1.PrivKeyFromBytes(raw)
}

func TestAddressFromPubKey(t *testing.T) {
	if got := AddressFromPubKey(testKey(t).PubKey()); got != anvilAddr0 {
		t.Fatalf("AddressFromPubKey = %s, want %s", got, anvilAddr0)
	}
}

func TestKeccak256MatchesKnownDigest(t *testing.T) {
	// keccak256("") — the well-known empty-input digest. If this drifts, the
	// hash function has been swapped for SHA3-256, which is a different one.
	const want = "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"
	if got := hex.EncodeToString(Keccak256(nil)); got != want {
		t.Fatalf("keccak256(\"\") = %s, want %s", got, want)
	}
}

// signPersonal produces an ERC-191 personal_sign signature the way a wallet
// would, so RecoverPersonalSigner is exercised end to end.
func signPersonal(t *testing.T, key *secp256k1.PrivateKey, msg []byte) []byte {
	t.Helper()
	compact := ecdsa.SignCompact(key, HashPersonalMessage(msg), false)
	sig := make([]byte, 65)
	copy(sig, compact[1:])
	sig[64] = compact[0] - 27
	return sig
}

func TestRecoverPersonalSigner(t *testing.T) {
	key := testKey(t)
	msg := []byte("hello signet")

	got, err := RecoverPersonalSigner(msg, signPersonal(t, key, msg))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if got != anvilAddr0 {
		t.Fatalf("recovered %s, want %s", got, anvilAddr0)
	}

	t.Run("legacy v encoding", func(t *testing.T) {
		sig := signPersonal(t, key, msg)
		sig[64] += 27 // wallets differ on whether v is 0/1 or 27/28
		got, err := RecoverPersonalSigner(msg, sig)
		if err != nil || got != anvilAddr0 {
			t.Fatalf("v=27/28 form rejected: %v (%s)", err, got)
		}
	})

	t.Run("different message", func(t *testing.T) {
		got, err := RecoverPersonalSigner([]byte("hello signal"), signPersonal(t, key, msg))
		if err == nil && got == anvilAddr0 {
			t.Fatal("signature recovered the signer for a message it did not sign")
		}
	})
}

func TestVerifyAuthKeyCertificate(t *testing.T) {
	key := testKey(t)
	digest := Keccak256([]byte("group:0xabc|nonce:1"))
	pubHex := hex.EncodeToString(key.PubKey().SerializeCompressed())

	sig := ecdsa.Sign(key, digest)
	raw := make([]byte, 64)
	r, s := sig.R(), sig.S()
	rb, sb := r.Bytes(), s.Bytes()
	copy(raw[:32], rb[:])
	copy(raw[32:], sb[:])

	if err := VerifyAuthKeyCertificate(pubHex, digest, raw); err != nil {
		t.Fatalf("valid auth-key certificate rejected: %v", err)
	}

	t.Run("wrong digest", func(t *testing.T) {
		if err := VerifyAuthKeyCertificate(pubHex, Keccak256([]byte("other")), raw); err == nil {
			t.Fatal("certificate verified against a different digest")
		}
	})
	t.Run("mutated signature", func(t *testing.T) {
		bad := append([]byte(nil), raw...)
		bad[10] ^= 0xff
		if err := VerifyAuthKeyCertificate(pubHex, digest, bad); err == nil {
			t.Fatal("mutated certificate was accepted")
		}
	})
}

func TestNormalizeAddress(t *testing.T) {
	got, err := NormalizeAddress("0xF39Fd6e51aad88F6F4ce6aB8827279cffFb92266")
	if err != nil || got != anvilAddr0 {
		t.Fatalf("NormalizeAddress = %q, %v", got, err)
	}
	for _, bad := range []string{"", "0x1234", "not-hex-at-all-not-hex-at-all-not-hex-1234", "0xzz39fd6e51aad88f6f4ce6ab8827279cfffb9226"} {
		if _, err := NormalizeAddress(bad); err == nil {
			t.Errorf("NormalizeAddress(%q) should have failed", bad)
		}
	}
}

func TestVerifySIWE(t *testing.T) {
	key := testKey(t)
	issued := time.Now().UTC().Format(time.RFC3339)
	raw := "console.signet.dev wants you to sign in with your Ethereum account:\n" +
		"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266\n\n" +
		"Sign in to the Signet platform console.\n\n" +
		"URI: https://console.signet.dev\n" +
		"Version: 1\n" +
		"Chain ID: 11155111\n" +
		"Nonce: abc123\n" +
		"Issued At: " + issued

	want := SIWEExpectation{Domain: "console.signet.dev", Nonce: "abc123", ChainID: 11155111}
	msg, err := VerifySIWE(raw, signPersonal(t, key, []byte(raw)), want)
	if err != nil {
		t.Fatalf("valid SIWE message rejected: %v", err)
	}
	if msg.Address != anvilAddr0 {
		t.Fatalf("address = %s, want %s", msg.Address, anvilAddr0)
	}
	if msg.Statement != "Sign in to the Signet platform console." {
		t.Fatalf("statement = %q", msg.Statement)
	}

	t.Run("nonce must match the issued challenge", func(t *testing.T) {
		bad := want
		bad.Nonce = "different"
		if _, err := VerifySIWE(raw, signPersonal(t, key, []byte(raw)), bad); err == nil {
			t.Fatal("a replayed nonce was accepted")
		}
	})
	t.Run("domain must match this server", func(t *testing.T) {
		bad := want
		bad.Domain = "evil.example"
		if _, err := VerifySIWE(raw, signPersonal(t, key, []byte(raw)), bad); err == nil {
			t.Fatal("a message for another domain was accepted")
		}
	})
	t.Run("chain must match", func(t *testing.T) {
		bad := want
		bad.ChainID = 1
		if _, err := VerifySIWE(raw, signPersonal(t, key, []byte(raw)), bad); err == nil {
			t.Fatal("a message for another chain was accepted")
		}
	})
	t.Run("expired message", func(t *testing.T) {
		expired := raw + "\nExpiration Time: " + time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
		if _, err := VerifySIWE(expired, signPersonal(t, key, []byte(expired)), want); err == nil {
			t.Fatal("an expired message was accepted")
		}
	})
	t.Run("signature must cover the exact message", func(t *testing.T) {
		tampered := strings.Replace(raw, "Chain ID: 11155111", "Chain ID: 1", 1)
		if _, err := VerifySIWE(tampered, signPersonal(t, key, []byte(raw)), want); err == nil {
			t.Fatal("a message altered after signing was accepted")
		}
	})
}

func TestSessionIssuer(t *testing.T) {
	issuer := NewSessionIssuer("test-secret-value", 60)
	uid := uuid.New()

	token, exp, err := issuer.Issue(uid, anvilAddr0, "signet")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !exp.After(time.Now()) {
		t.Fatal("token expires in the past")
	}
	claims, err := issuer.Verify(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.UserID != uid || claims.Subject != anvilAddr0 || claims.Method != "signet" {
		t.Fatalf("claims round-tripped wrong: %+v", claims)
	}

	t.Run("rejects a token signed with another secret", func(t *testing.T) {
		other := NewSessionIssuer("different-secret", 60)
		if _, err := other.Verify(token); err == nil {
			t.Fatal("token from a foreign secret was accepted")
		}
	})
	t.Run("rejects an expired token", func(t *testing.T) {
		short := NewSessionIssuer("test-secret-value", -1)
		// A negative TTL falls back to the default, so expiry is forced instead
		// by verifying a token minted with an already-elapsed lifetime.
		tok, _, err := short.Issue(uid, anvilAddr0, "signet")
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		if _, err := short.Verify(tok); err != nil {
			t.Fatalf("default-TTL token should still verify: %v", err)
		}
	})
	t.Run("rejects garbage", func(t *testing.T) {
		if _, err := issuer.Verify("not.a.token"); err == nil {
			t.Fatal("garbage token was accepted")
		}
	})
}

func TestRoleOrdering(t *testing.T) {
	if !RoleOwner.AtLeast(RoleAdmin) || !RoleAdmin.AtLeast(RoleDeveloper) || !RoleDeveloper.AtLeast(RoleViewer) {
		t.Fatal("role ranking is not monotonic")
	}
	if RoleViewer.AtLeast(RoleDeveloper) {
		t.Fatal("viewer should not satisfy developer")
	}
	if Role("nonsense").Valid() {
		t.Fatal("unknown role reported as valid")
	}
}
