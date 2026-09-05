package api

import "testing"

// A group the console created answers to the smart wallet; one created from a
// developer's own wallet answers to their EOA. Both are theirs. Anything else
// is not, and accepting it would let someone bind an app to a group they
// cannot control — and then show them buttons that always revert.
func TestManagerMatches(t *testing.T) {
	const smart = "0xAAAAaaaaAAAAaaaaAAAAaaaaAAAAaaaaAAAAaaaa"
	const eoa = "0xBBBBbbbbBBBBbbbbBBBBbbbbBBBBbbbbBBBBbbbb"
	const stranger = "0xCCCCccccCCCCccccCCCCccccCCCCccccCCCCcccc"

	cases := []struct {
		name    string
		manager string
		addrs   []*string
		want    bool
	}{
		{"smart wallet", smart, []*string{ptr(smart), ptr(eoa)}, true},
		{"own eoa", eoa, []*string{ptr(smart), ptr(eoa)}, true},
		{"case insensitive", "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", []*string{ptr(smart)}, true},
		{"stranger", stranger, []*string{ptr(smart), ptr(eoa)}, false},
		{"no addresses", smart, nil, false},
		{"nil address", smart, []*string{nil}, false},
		// An account with no smart wallet yet must not match a group whose
		// manager field came back empty.
		{"empty address", "", []*string{ptr("")}, false},
		{"empty manager", "", []*string{ptr(smart)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := managerMatches(tc.manager, tc.addrs...); got != tc.want {
				t.Fatalf("managerMatches(%q, %v) = %v, want %v", tc.manager, tc.addrs, got, tc.want)
			}
		})
	}
}
