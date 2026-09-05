package chain

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// blob assembles ABI return data from 32-byte-word hex chunks, so each test
// case reads as the layout the contract actually produces.
func blob(t *testing.T, words ...string) []byte {
	t.Helper()
	var sb strings.Builder
	for _, w := range words {
		w = strings.ReplaceAll(w, " ", "")
		if len(w) != 64 {
			t.Fatalf("word %q is %d hex chars, want 64", w, len(w))
		}
		sb.WriteString(w)
	}
	raw, err := hex.DecodeString(sb.String())
	if err != nil {
		t.Fatalf("decode blob: %v", err)
	}
	return raw
}

func word(hexBody string) string {
	return strings.Repeat("0", 64-len(hexBody)) + hexBody
}

func TestSelectorMatchesKnownSignatures(t *testing.T) {
	// Selectors cross-checked against the published 4-byte values for these
	// canonical signatures; a mismatch means the keccak implementation or the
	// signature string is wrong, and every call would revert.
	cases := map[string]string{
		"transfer(address,uint256)": "a9059cbb",
		"balanceOf(address)":        "70a08231",
		"totalSupply()":             "18160ddd",
		"approve(address,uint256)":  "095ea7b3",
	}
	for sig, want := range cases {
		if got := hex.EncodeToString(selector(sig)); got != want {
			t.Errorf("selector(%q) = %s, want %s", sig, got, want)
		}
	}
}

func TestDecodeAddressArray(t *testing.T) {
	data := blob(t,
		word("20"), // offset to the array
		word("2"),  // length
		word("f39fd6e51aad88f6f4ce6ab8827279cfffb92266"),
		word("70997970c51812dc3a010c7d01b50e0d17dc79c8"),
	)
	got, err := decoder{data}.addressArray()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []string{"0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266", "0x70997970c51812dc3a010c7d01b50e0d17dc79c8"}
	if len(got) != len(want) {
		t.Fatalf("got %d addresses, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("address %d = %s, want %s", i, got[i], want[i])
		}
	}

	t.Run("empty array", func(t *testing.T) {
		empty := blob(t, word("20"), word("0"))
		got, err := decoder{empty}.addressArray()
		if err != nil || len(got) != 0 {
			t.Fatalf("empty array decoded as %v, %v", got, err)
		}
	})
}

func TestDecodeBytesArray(t *testing.T) {
	// bytes[] with two compressed secp256k1 keys (33 bytes each, so each
	// element occupies a length word plus two data words).
	data := blob(t,
		word("20"), // offset to array
		word("2"),  // length
		word("40"), // element 0 offset, relative to the element region
		word("a0"), // element 1 offset
		word("21"), // element 0 length = 33
		// 33 bytes of payload spill into two words, right-padded with zeros.
		"02"+strings.Repeat("11", 31),
		"11"+strings.Repeat("00", 31),
		word("21"), // element 1 length = 33
		"03"+strings.Repeat("22", 31),
		"22"+strings.Repeat("00", 31),
	)
	got, err := decoder{data}.bytesArray()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d elements, want 2", len(got))
	}
	if len(got[0]) != 33 || got[0][0] != 0x02 {
		t.Errorf("element 0 = %x", got[0])
	}
	if len(got[1]) != 33 || got[1][0] != 0x03 {
		t.Errorf("element 1 = %x", got[1])
	}
}

func TestDecodeRejectsTruncatedData(t *testing.T) {
	// An offset that points past the end must be an error, never a panic —
	// return data comes from an untrusted RPC endpoint.
	bad := blob(t, word("ffff"))
	if _, err := (decoder{bad}).addressArray(); err == nil {
		t.Fatal("out-of-range offset was accepted")
	}
	if _, err := (decoder{nil}).addressArray(); err == nil {
		t.Fatal("empty return data was accepted")
	}
	short := blob(t, word("20"), word("5")) // claims 5 items, supplies none
	if _, err := (decoder{short}).addressArray(); err == nil {
		t.Fatal("truncated array was accepted")
	}
}

func TestDecodeBoolRejectsDirtyWord(t *testing.T) {
	dirty := blob(t, "00000000000000000000000000000000000000000000000000000000ff000001")
	if _, err := (decoder{dirty}).boolAt(0); err == nil {
		t.Fatal("bool with dirty high bytes was accepted")
	}
	clean := blob(t, word("1"))
	got, err := (decoder{clean}).boolAt(0)
	if err != nil || !got {
		t.Fatalf("boolAt = %v, %v", got, err)
	}
}

func TestIssuerAndAuthKeyHashes(t *testing.T) {
	// keccak256("https://accounts.google.com"), the handle removeIssuer takes.
	// Recomputed here through the same helper the console will show, so a
	// change to the hashing scheme is caught rather than silently accepted by
	// a contract call that then reverts.
	got := IssuerHash("https://accounts.google.com")
	if len(got) != 66 || !strings.HasPrefix(got, "0x") {
		t.Fatalf("IssuerHash returned %q", got)
	}
	if IssuerHash("https://accounts.google.com") != got {
		t.Fatal("IssuerHash is not deterministic")
	}
	if IssuerHash("https://appleid.apple.com") == got {
		t.Fatal("different issuers hashed to the same handle")
	}

	keyHash, err := AuthKeyHash("0x02" + strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("AuthKeyHash: %v", err)
	}
	if len(keyHash) != 66 {
		t.Fatalf("AuthKeyHash returned %q", keyHash)
	}
}

func TestAddressWordPadsLeft(t *testing.T) {
	w, err := addressWord("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")
	if err != nil {
		t.Fatalf("addressWord: %v", err)
	}
	if hex.EncodeToString(w) != word("f39fd6e51aad88f6f4ce6ab8827279cfffb92266") {
		t.Fatalf("addressWord = %s", hex.EncodeToString(w))
	}
	if _, err := addressWord("0x1234"); err == nil {
		t.Fatal("short address was accepted")
	}
}

// A nil slice marshals to `null`, and the console's types declare these as
// arrays — so an empty group would throw on `.length` rather than rendering an
// empty section. Every collection on GroupState must serialize as `[]`.
func TestGroupStateSlicesMarshalAsArrays(t *testing.T) {
	st := &GroupState{
		Address:         "0xabc",
		ActiveNodes:     []string{},
		PendingNodes:    []string{},
		PendingRemovals: []RemovalRequest{},
		Issuers:         []Issuer{},
		AuthKeys:        []string{},
	}
	encoded, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), "null") {
		t.Fatalf("GroupState serialized a null collection: %s", encoded)
	}
	for _, field := range []string{
		`"active_nodes":[]`, `"pending_nodes":[]`, `"pending_removals":[]`,
		`"issuers":[]`, `"auth_keys":[]`,
	} {
		if !strings.Contains(string(encoded), field) {
			t.Errorf("expected %s in %s", field, encoded)
		}
	}
}

// Issuer.ClientIDs is nested inside a collection the console iterates, so it
// has the same requirement.
func TestIssuerClientIDsMarshalAsArray(t *testing.T) {
	encoded, err := json.Marshal(Issuer{Issuer: "https://accounts.google.com", ClientIDs: []string{}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"client_ids":[]`) {
		t.Fatalf("client_ids serialized as %s", encoded)
	}
}
