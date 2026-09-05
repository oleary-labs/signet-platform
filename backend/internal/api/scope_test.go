package api

import (
	"strings"
	"testing"

	"github.com/oleary-labs/signet-platform/backend/internal/store"
)

func TestApplyScopeDecodesEIP712(t *testing.T) {
	// scheme 0x03 | chainId 8453 (Base) | USDC | a TransferWithAuthorization
	// type hash. 61 bytes total, per DESIGN-SCOPED-SUBKEYS.
	scope := "03" +
		"0000000000002105" +
		"833589fcd6edb6e08f4c7c32d4f71b54bda02913" +
		strings.Repeat("ab", 32)

	var rec store.KeyRecord
	if err := applyScope(&rec, scope); err != nil {
		t.Fatalf("valid EIP-712 scope rejected: %v", err)
	}
	if rec.ScopeKind != "eip712" {
		t.Fatalf("scope kind = %q, want eip712", rec.ScopeKind)
	}
	if rec.ScopeChainID == nil || *rec.ScopeChainID != 8453 {
		t.Fatalf("chain id = %v, want 8453", rec.ScopeChainID)
	}
	if rec.ScopeContract != "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913" {
		t.Fatalf("contract = %q", rec.ScopeContract)
	}
	if rec.ScopeTypeHash != "0x"+strings.Repeat("ab", 32) {
		t.Fatalf("type hash = %q", rec.ScopeTypeHash)
	}
}

func TestApplyScopeDecodesUserOp(t *testing.T) {
	// scheme 0x01 | entryPoint(20) | chainId(8) | sender(20) = 49 bytes.
	scope := "01" + strings.Repeat("11", 20) + "0000000000000001" + strings.Repeat("22", 20)
	var rec store.KeyRecord
	if err := applyScope(&rec, scope); err != nil {
		t.Fatalf("valid UserOp scope rejected: %v", err)
	}
	if rec.ScopeKind != "evm_userop" {
		t.Fatalf("scope kind = %q, want evm_userop", rec.ScopeKind)
	}
	if rec.ScopeChainID == nil || *rec.ScopeChainID != 1 {
		t.Fatalf("chain id = %v, want 1", rec.ScopeChainID)
	}
	if rec.ScopeContract != "0x"+strings.Repeat("22", 20) {
		t.Fatalf("sender = %q", rec.ScopeContract)
	}
}

func TestApplyScopeUnscoped(t *testing.T) {
	for _, in := range []string{"", "00", "0x00"} {
		var rec store.KeyRecord
		if err := applyScope(&rec, in); err != nil {
			t.Fatalf("applyScope(%q) errored: %v", in, err)
		}
		if rec.ScopeKind != "unscoped" {
			t.Fatalf("applyScope(%q) = %q, want unscoped", in, rec.ScopeKind)
		}
	}
}

// A malformed scope must fail loudly. Falling back to "unscoped" would show a
// constrained key as unrestricted, which is the one thing this screen exists
// to get right.
func TestApplyScopeRejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"odd-length hex":    "03" + "0000000000002105" + strings.Repeat("a", 61),
		"non-hex":           "03zzzz",
		"truncated eip712":  "03" + "0000000000002105",
		"overlong eip712":   "03" + "0000000000002105" + strings.Repeat("ab", 60),
		"truncated userop":  "01" + strings.Repeat("11", 20),
		"unknown scheme":    "09" + strings.Repeat("11", 20),
		"wrong-size solana": "02" + strings.Repeat("11", 20),
	}
	for name, scope := range cases {
		var rec store.KeyRecord
		if err := applyScope(&rec, scope); err == nil {
			t.Errorf("%s: malformed scope was accepted as %q", name, rec.ScopeKind)
		}
	}
}

func TestSubjectHashGroupsAUsersKeys(t *testing.T) {
	root := "oauth:https://accounts.google.com:1029384756"
	sub := root + ":a1b2c3d4e5f60718"

	rootHash := subjectHashFromKeyID(root)
	subHash := subjectHashFromKeyID(sub)
	if rootHash == "" {
		t.Fatal("root key produced no subject hash")
	}
	if rootHash != subHash {
		t.Fatalf("a scoped sub-key hashed to a different user than its parent:\n  root %s\n  sub  %s", rootHash, subHash)
	}
	// The hash must not be reversible to the subject by inspection.
	if strings.Contains(rootHash, "1029384756") {
		t.Fatal("subject hash leaks the raw OAuth subject")
	}
	// A different user must not collide.
	if subjectHashFromKeyID("oauth:https://accounts.google.com:999") == rootHash {
		t.Fatal("two different subjects hashed to the same user")
	}
	// A key ID that is not auth-derived has no user.
	if subjectHashFromKeyID("k1") != "" {
		t.Fatal("a non-OAuth key ID produced a subject hash")
	}
}

func TestParentKeyID(t *testing.T) {
	root := "oauth:https://accounts.google.com:1029384756"
	if got := parentKeyID(root + ":a1b2c3d4e5f60718"); got != root {
		t.Fatalf("parentKeyID = %q, want %q", got, root)
	}
	if got := parentKeyID(root); got != "" {
		t.Fatalf("a root key reported parent %q", got)
	}
	// A trailing segment that is not a 16-hex scope hash is part of the
	// subject, not a scope suffix.
	if got := parentKeyID("oauth:https://accounts.google.com:not-a-hash"); got != "" {
		t.Fatalf("a non-suffix tail was read as a scope suffix: %q", got)
	}
}

func TestNormalizeOrigin(t *testing.T) {
	cases := map[string]string{
		"https://app.example.com":  "https://app.example.com",
		"app.example.com":          "https://app.example.com",
		"http://localhost:3000":    "http://localhost:3000",
		"https://app.example.com/": "https://app.example.com",
	}
	for in, want := range cases {
		got, err := normalizeOrigin(in)
		if err != nil || got != want {
			t.Errorf("normalizeOrigin(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "https://app.example.com/callback", "https://"} {
		if _, err := normalizeOrigin(bad); err == nil {
			t.Errorf("normalizeOrigin(%q) should have failed", bad)
		}
	}
}
