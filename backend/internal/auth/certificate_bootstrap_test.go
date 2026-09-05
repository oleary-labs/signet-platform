package auth

import "testing"

// boot.sh derives the auth key's public half in shell (compressing the point by
// the parity of y) and registers that with addAuthKey; this package derives it
// again in Go and presents it to the nodes. If the two ever disagree the nodes
// reject every certificate with "untrusted authorization key" — a failure that
// surfaces at signup, far from its cause. So the pair is pinned here.
//
// The constant below is boot.sh's default development key. Changing that
// default should fail this test.
func TestBootScriptPublicKeyMatchesSigner(t *testing.T) {
	const priv = "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	const fromBootSh = "0x0002ba5734d8f7091719471e7f7ed6b9df170dc70cc661ca05e688601ad984f068b0"

	s, err := NewCertificateSigner(priv, "0xgroup", 0)
	if err != nil {
		t.Fatalf("NewCertificateSigner: %v", err)
	}
	if s == nil {
		t.Fatal("signer is nil")
	}
	if s.PublicKey() != fromBootSh {
		t.Fatalf("signer public key = %s\nboot.sh registers  = %s", s.PublicKey(), fromBootSh)
	}
}
