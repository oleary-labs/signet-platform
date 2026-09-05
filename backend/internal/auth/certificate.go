package auth

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// AuthKeyCertificate is the credential a node accepts on POST /v1/auth in place
// of a ZK proof. It binds a logical identity to an ephemeral session key, and
// is signed by a key the group trusts on-chain.
type AuthKeyCertificate struct {
	Identity   string `json:"identity"`
	Expiry     int64  `json:"expiry"`
	AuthKeyPub string `json:"auth_key_pub"`
	Signature  string `json:"signature"`
}

// CertificateSigner issues those certificates.
//
// The platform is the identity provider for its own console: it verifies a
// developer however they signed in — a ZK proof of OAuth, or a SIWE
// signature — and then vouches for them to its own signing group. That is what
// lets a wallet user hold a Signet key without ever touching MetaMask again.
//
// The blast radius is worth stating plainly: whoever holds this key can mint a
// session for any identity in the platform's group. It is the platform's own
// group, holding the platform's own users' console keys, and nothing else
// trusts it — but it is a hot key and should be treated as one.
type CertificateSigner struct {
	key      *secp256k1.PrivateKey
	pubHex   string
	groupID  string
	lifetime time.Duration
}

// NewCertificateSigner builds a signer. An empty key returns (nil, nil): the
// platform then cannot vouch for anyone, which callers report rather than
// crashing on.
func NewCertificateSigner(privateKeyHex, groupID string, lifetime time.Duration) (*CertificateSigner, error) {
	if strings.TrimSpace(privateKeyHex) == "" || strings.TrimSpace(groupID) == "" {
		return nil, nil
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(privateKeyHex), "0x"))
	if err != nil {
		return nil, fmt.Errorf("platform auth key is not hex: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("platform auth key must be 32 bytes, got %d", len(raw))
	}
	key := secp256k1.PrivKeyFromBytes(raw)
	if lifetime <= 0 {
		lifetime = time.Hour
	}
	return &CertificateSigner{
		key: key,
		// 0x00 is the ECDSA scheme prefix; the group stores auth keys in this
		// 34-byte scheme-prefixed form, and a bare 33-byte key will not match.
		pubHex:   "0x00" + hex.EncodeToString(key.PubKey().SerializeCompressed()),
		groupID:  strings.ToLower(groupID),
		lifetime: lifetime,
	}, nil
}

// PublicKey is the scheme-prefixed public half, which must be registered on the
// group with addAuthKey before any certificate it signs will be accepted.
func (c *CertificateSigner) PublicKey() string { return c.pubHex }

// GroupID is the group these certificates are valid for.
func (c *CertificateSigner) GroupID() string { return c.groupID }

// Issue signs a certificate binding identity to sessionPubHex.
func (c *CertificateSigner) Issue(identity, sessionPubHex string) (*AuthKeyCertificate, error) {
	if identity == "" {
		return nil, fmt.Errorf("identity is required")
	}
	sessionPubHex = strings.ToLower(strings.TrimPrefix(sessionPubHex, "0x"))
	if _, err := hex.DecodeString(sessionPubHex); err != nil || len(sessionPubHex) != 66 {
		return nil, fmt.Errorf("session public key must be 33 compressed bytes of hex")
	}
	expiry := time.Now().Add(c.lifetime).Unix()

	sig := ecdsa.Sign(c.key, certificateHash(identity, c.groupID, sessionPubHex, expiry))
	// The node expects 64 bytes of [R || S] with no recovery byte. dcrd
	// normalizes S to the low half already, which is what the verifier wants.
	r, s := sig.R(), sig.S()
	rb, sb := r.Bytes(), s.Bytes()
	raw := make([]byte, 64)
	copy(raw[:32], rb[:])
	copy(raw[32:], sb[:])

	return &AuthKeyCertificate{
		Identity:   identity,
		Expiry:     expiry,
		AuthKeyPub: c.pubHex,
		Signature:  hex.EncodeToString(raw),
	}, nil
}

// certificateHash is SHA256(identity : group_id : session_pub_hex : expiry_be64),
// matching what signetd recomputes before it accepts the signature.
func certificateHash(identity, groupID, sessionPubHex string, expiry int64) []byte {
	var expiryBytes [8]byte
	binary.BigEndian.PutUint64(expiryBytes[:], uint64(expiry))

	h := sha256.New()
	h.Write([]byte(identity))
	h.Write([]byte(":"))
	h.Write([]byte(groupID))
	h.Write([]byte(":"))
	h.Write([]byte(sessionPubHex))
	h.Write([]byte(":"))
	h.Write(expiryBytes[:])
	return h.Sum(nil)
}
