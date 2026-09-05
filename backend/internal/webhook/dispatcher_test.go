package webhook

import (
	"strings"
	"testing"
)

func TestSignatureBindsTimestampAndBody(t *testing.T) {
	body := []byte(`{"event":"key.disabled"}`)
	sig := Sign("whsec_test", "1700000000", body)

	if !strings.HasPrefix(sig, "v1=") {
		t.Fatalf("signature %q is missing its version prefix", sig)
	}
	if !Verify("whsec_test", "1700000000", body, sig) {
		t.Fatal("a signature did not verify against its own inputs")
	}

	// The timestamp is inside the signed material, so a captured delivery
	// cannot be replayed later under a fresh timestamp.
	if Verify("whsec_test", "1700009999", body, sig) {
		t.Fatal("signature verified under a different timestamp")
	}
	if Verify("whsec_test", "1700000000", []byte(`{"event":"key.enabled"}`), sig) {
		t.Fatal("signature verified over a different body")
	}
	if Verify("whsec_other", "1700000000", body, sig) {
		t.Fatal("signature verified under a different secret")
	}
}

func TestSignatureIsDeterministic(t *testing.T) {
	body := []byte("payload")
	if Sign("s", "1", body) != Sign("s", "1", body) {
		t.Fatal("signing the same inputs produced different signatures")
	}
}
