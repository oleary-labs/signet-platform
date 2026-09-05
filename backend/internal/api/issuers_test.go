package api

import "testing"

// The on-chain call is skipped only when the allow-list is genuinely unchanged.
// Getting this wrong in the permissive direction would let a developer widen
// which OAuth clients are accepted without the operators ever seeing it.
func TestSameClientIDs(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"identical", []string{"a", "b"}, []string{"a", "b"}, true},
		{"reordered", []string{"b", "a"}, []string{"a", "b"}, true},
		{"both empty", []string{}, []string{}, true},
		{"nil and empty", nil, []string{}, true},
		{"added one", []string{"a"}, []string{"a", "b"}, false},
		{"removed one", []string{"a", "b"}, []string{"a"}, false},
		{"swapped value", []string{"a"}, []string{"b"}, false},
		// "allow any client" versus "allow one client" is the widest possible
		// change, and must never read as unchanged.
		{"empty to one", []string{}, []string{"a"}, false},
		{"one to empty", []string{"a"}, []string{}, false},
		{"duplicates differ", []string{"a", "a"}, []string{"a", "b"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameClientIDs(tc.a, tc.b); got != tc.want {
				t.Fatalf("sameClientIDs(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
