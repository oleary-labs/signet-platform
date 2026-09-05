package auth

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
)

// Keccak256 returns the Keccak-256 digest used throughout Ethereum. Note this
// is the original Keccak padding, not the later SHA3-256 standard — they are
// different functions and are not interchangeable here.
func Keccak256(parts ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}

// AddressFromPubKey derives the 20-byte Ethereum address of a public key:
// the low 20 bytes of keccak256 over the uncompressed key with its 0x04
// prefix stripped.
func AddressFromPubKey(pub *secp256k1.PublicKey) string {
	uncompressed := pub.SerializeUncompressed()
	return "0x" + hex.EncodeToString(Keccak256(uncompressed[1:])[12:])
}

// ethPersonalPrefix is the ERC-191 personal_sign framing. Wallets always sign
// the prefixed digest, so a signature can never be replayed as a transaction.
const ethPersonalPrefix = "\x19Ethereum Signed Message:\n"

// HashPersonalMessage computes the ERC-191 personal_sign digest of msg.
func HashPersonalMessage(msg []byte) []byte {
	return Keccak256([]byte(fmt.Sprintf("%s%d", ethPersonalPrefix, len(msg))), msg)
}

// RecoverPersonalSigner recovers the address that produced an ERC-191
// personal_sign signature over msg. sig is 65 bytes r || s || v, with v either
// 0/1 or 27/28 — both are seen in the wild and both are accepted.
func RecoverPersonalSigner(msg, sig []byte) (string, error) {
	if len(sig) != 65 {
		return "", fmt.Errorf("%w: signature must be 65 bytes, got %d", ErrInvalidSignature, len(sig))
	}
	v := sig[64]
	if v >= 27 {
		v -= 27
	}
	if v > 1 {
		return "", fmt.Errorf("%w: recovery id out of range", ErrInvalidSignature)
	}

	// dcrd's compact encoding puts the recovery byte first and encodes
	// "was the pubkey compressed" as +4; Ethereum always recovers the
	// uncompressed key, so the flag stays clear.
	compact := make([]byte, 65)
	compact[0] = 27 + v
	copy(compact[1:], sig[:64])

	pub, _, err := ecdsa.RecoverCompact(compact, HashPersonalMessage(msg))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}
	return AddressFromPubKey(pub), nil
}

// VerifyAuthKeyCertificate checks a 64-byte [R||S] ECDSA signature by a
// group authorization key over a 32-byte digest. This is the same credential
// shape signetd accepts on /v1/auth and POST /admin/keys, so a developer can
// use one application key for both the nodes and the platform API.
func VerifyAuthKeyCertificate(pubKeyHex string, digest, sig []byte) error {
	if len(sig) != 64 {
		return fmt.Errorf("%w: auth-key signature must be 64 bytes, got %d", ErrInvalidSignature, len(sig))
	}
	if len(digest) != 32 {
		return fmt.Errorf("%w: digest must be 32 bytes, got %d", ErrInvalidSignature, len(digest))
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(pubKeyHex), "0x"))
	if err != nil {
		return fmt.Errorf("%w: auth key is not hex", ErrInvalidSignature)
	}
	pub, err := secp256k1.ParsePubKey(raw)
	if err != nil {
		return fmt.Errorf("%w: auth key is not a valid point", ErrInvalidSignature)
	}

	var r, s secp256k1.ModNScalar
	if overflow := r.SetByteSlice(sig[:32]); overflow || r.IsZero() {
		return fmt.Errorf("%w: r out of range", ErrInvalidSignature)
	}
	if overflow := s.SetByteSlice(sig[32:]); overflow || s.IsZero() {
		return fmt.Errorf("%w: s out of range", ErrInvalidSignature)
	}
	if !ecdsa.NewSignature(&r, &s).Verify(digest, pub) {
		return fmt.Errorf("%w: auth-key signature does not verify", ErrInvalidSignature)
	}
	return nil
}

// NormalizeAddress lower-cases and 0x-prefixes an Ethereum address, and
// rejects anything that is not 20 bytes of hex. Addresses are compared as
// strings all over the platform, so they must be normalized on the way in.
func NormalizeAddress(addr string) (string, error) {
	trimmed := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(addr)), "0x")
	if len(trimmed) != 40 {
		return "", fmt.Errorf("address must be 20 bytes of hex")
	}
	if _, err := hex.DecodeString(trimmed); err != nil {
		return "", fmt.Errorf("address is not hex: %w", err)
	}
	return "0x" + trimmed, nil
}

// DecodeHex accepts an optionally 0x-prefixed hex string.
func DecodeHex(s string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(s), "0x"))
}
