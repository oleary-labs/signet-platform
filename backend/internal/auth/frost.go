// Package auth implements the platform's login routes and org authorization.
//
// The platform dogfoods Signet: a developer proves control of their
// SignetAccount by having the bootstrap signing group threshold-sign a
// server-issued challenge, and this package verifies that FROST Schnorr
// signature server-side. An EOA route (SIWE) is offered alongside it for node
// operators and teams that have not onboarded through Signet yet.
package auth

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// frostChallengeDST is the RFC 9591 domain separation tag for the challenge
// hash of the FROST-secp256k1-SHA256-v1 ciphersuite. It must match the tag the
// nodes, the SDK (frostVerify.ts), and FROSTVerifier.sol use, or every
// signature verifies as invalid.
var frostChallengeDST = []byte("FROST-secp256k1-SHA256-v1chal")

// curveOrder is the secp256k1 group order n, used to reduce the 384-bit
// challenge output at full width.
var curveOrder = secp256k1.S256().N

// ErrInvalidSignature is returned when a signature does not verify. It is
// deliberately opaque: callers must not branch on *why* verification failed.
var ErrInvalidSignature = errors.New("invalid signature")

// VerifyFROST verifies a 65-byte FROST threshold Schnorr signature over
// message under the compressed group public key.
//
// Signature layout is R.x (32) || z (32) || v (1), where v is the parity of
// R.y — the same wire format signetd returns as `ethereum_signature` and the
// same one SignetAccount's validator consumes on-chain.
//
// The verification equation is z·G == R + c·Y with
// c = H2(R_compressed || Y_compressed || message), where H2 is
// expand_message_xmd(SHA-256) to 48 bytes reduced mod n (RFC 9380 §5.3).
func VerifyFROST(message, signature, groupPublicKey []byte) error {
	if len(signature) != 65 {
		return fmt.Errorf("%w: signature must be 65 bytes, got %d", ErrInvalidSignature, len(signature))
	}
	if len(groupPublicKey) != 33 {
		return fmt.Errorf("%w: group public key must be 33 compressed bytes, got %d", ErrInvalidSignature, len(groupPublicKey))
	}

	// Reconstruct the compressed R point from its x-coordinate and parity.
	rCompressed := make([]byte, 33)
	if signature[64] == 0 {
		rCompressed[0] = 0x02
	} else if signature[64] == 1 {
		rCompressed[0] = 0x03
	} else {
		return fmt.Errorf("%w: parity byte must be 0 or 1, got %d", ErrInvalidSignature, signature[64])
	}
	copy(rCompressed[1:], signature[0:32])

	var z secp256k1.ModNScalar
	// Overflow means z >= n, which is not a valid scalar. A zero z is likewise
	// rejected — it would make the equation independent of the nonce.
	if overflow := z.SetByteSlice(signature[32:64]); overflow || z.IsZero() {
		return fmt.Errorf("%w: z is not a valid scalar", ErrInvalidSignature)
	}

	rPub, err := secp256k1.ParsePubKey(rCompressed)
	if err != nil {
		return fmt.Errorf("%w: R is not on the curve", ErrInvalidSignature)
	}
	yPub, err := secp256k1.ParsePubKey(groupPublicKey)
	if err != nil {
		return fmt.Errorf("%w: group public key is not on the curve", ErrInvalidSignature)
	}

	// c = H2(R || Y || msg), reduced mod n.
	input := make([]byte, 0, 33+33+len(message))
	input = append(input, rCompressed...)
	input = append(input, groupPublicKey...)
	input = append(input, message...)

	uniform, err := expandMessageXMD(input, frostChallengeDST, 48)
	if err != nil {
		return fmt.Errorf("expand challenge: %w", err)
	}
	// The uniform bytes are a 384-bit integer, so the reduction mod n has to
	// happen at full width. ModNScalar.SetByteSlice truncates anything past 32
	// bytes rather than reducing it, which would silently produce a different
	// challenge from the one the nodes and FROSTVerifier.sol compute.
	cInt := new(big.Int).SetBytes(uniform)
	cInt.Mod(cInt, curveOrder)
	var cBytes [32]byte
	cInt.FillBytes(cBytes[:])
	var c secp256k1.ModNScalar
	if overflow := c.SetBytes(&cBytes); overflow == 1 || c.IsZero() {
		return fmt.Errorf("%w: challenge is not a valid scalar", ErrInvalidSignature)
	}

	// lhs = z·G
	var lhs secp256k1.JacobianPoint
	secp256k1.ScalarBaseMultNonConst(&z, &lhs)

	// rhs = R + c·Y
	var yJ, cY, rJ, rhs secp256k1.JacobianPoint
	yPub.AsJacobian(&yJ)
	rPub.AsJacobian(&rJ)
	secp256k1.ScalarMultNonConst(&c, &yJ, &cY)
	secp256k1.AddNonConst(&rJ, &cY, &rhs)

	lhs.ToAffine()
	rhs.ToAffine()
	if !lhs.X.Equals(&rhs.X) || !lhs.Y.Equals(&rhs.Y) {
		return fmt.Errorf("%w: verification equation does not hold", ErrInvalidSignature)
	}
	return nil
}

// expandMessageXMD implements RFC 9380 §5.3.1 expand_message_xmd with SHA-256.
// It is the hash-to-field expansion the FROST ciphersuite specifies for its
// challenge; a plain SHA-256 would produce a different challenge and every
// signature would fail to verify.
func expandMessageXMD(msg, dst []byte, lenInBytes int) ([]byte, error) {
	const bInBytes = sha256.Size // hash output size
	const sInBytes = 64          // SHA-256 block size

	if len(dst) > 255 {
		return nil, fmt.Errorf("DST longer than 255 bytes")
	}
	ell := (lenInBytes + bInBytes - 1) / bInBytes
	if ell > 255 || lenInBytes > 65535 {
		return nil, fmt.Errorf("requested length %d out of range", lenInBytes)
	}

	dstPrime := append(append([]byte{}, dst...), byte(len(dst)))

	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(lenInBytes))

	// b_0 = H(Z_pad || msg || l_i_b_str || I2OSP(0, 1) || DST_prime)
	h := sha256.New()
	h.Write(make([]byte, sInBytes))
	h.Write(msg)
	h.Write(lenBuf[:])
	h.Write([]byte{0})
	h.Write(dstPrime)
	b0 := h.Sum(nil)

	// b_1 = H(b_0 || I2OSP(1, 1) || DST_prime)
	h.Reset()
	h.Write(b0)
	h.Write([]byte{1})
	h.Write(dstPrime)
	bi := h.Sum(nil)

	out := make([]byte, 0, ell*bInBytes)
	out = append(out, bi...)

	// b_i = H(strxor(b_0, b_{i-1}) || I2OSP(i, 1) || DST_prime)
	for i := 2; i <= ell; i++ {
		xored := make([]byte, bInBytes)
		for j := range xored {
			xored[j] = b0[j] ^ bi[j]
		}
		h.Reset()
		h.Write(xored)
		h.Write([]byte{byte(i)})
		h.Write(dstPrime)
		bi = h.Sum(nil)
		out = append(out, bi...)
	}
	return out[:lenInBytes], nil
}
