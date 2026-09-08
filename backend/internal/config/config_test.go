package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// Every exported Config field must actually be assigned in Load(). A field that
// is declared but never populated compiles, passes vet, and silently disables
// whatever it controls — which is exactly how the bundler and the paymaster
// were dark for a while.
func TestEveryFieldIsAssignedInLoad(t *testing.T) {
	source, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatalf("read config.go: %v", err)
	}
	body := string(source)
	start := strings.Index(body, "c := &Config{")
	if start == -1 {
		t.Fatal("could not find the Config literal in Load()")
	}
	literal := body[start:]

	typ := reflect.TypeOf(Config{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if !strings.Contains(literal, name+":") {
			t.Errorf("Config.%s is declared but never assigned in Load()", name)
		}
	}
}

// Staff is gated on the sign-in subject rather than on users.email, which is
// an unverified profile string. These cases are the ones where a mismatch
// would be silent: the person is simply not staff, with nothing to read.
func TestStaffSubjectNormalisation(t *testing.T) {
	cfg := &Config{StaffSubjects: normalizeSubjects(splitList(
		" eth:0xDCB876Ac74297655a73F04DC7a5039175dD15e10 , 0xFF1dA08f644749E4bc56e1643453F3746c8A5E0E , Signet:03AB , nonsense , 0x1234 "))}

	want := []string{
		"eth:0xdcb876ac74297655a73f04dc7a5039175dd15e10", // checksummed, spaced
		"eth:0xff1da08f644749e4bc56e1643453f3746c8a5e0e", // bare address
		"signet:03ab",
	}
	if len(cfg.StaffSubjects) != len(want) {
		t.Fatalf("got %v, want %v", cfg.StaffSubjects, want)
	}
	for i, w := range want {
		if cfg.StaffSubjects[i] != w {
			t.Errorf("entry %d: got %q, want %q", i, cfg.StaffSubjects[i], w)
		}
	}

	// A block-explorer paste and the stored lower-case form are the same
	// subject; a different address is not.
	if !cfg.IsStaffSubject("eth:0xdcb876ac74297655a73f04dc7a5039175dd15e10") {
		t.Error("stored subject form should match")
	}
	if !cfg.IsStaffSubject("ETH:0xDCB876AC74297655A73F04DC7A5039175DD15E10") {
		t.Error("matching should be case-insensitive")
	}
	if cfg.IsStaffSubject("eth:0x0000000000000000000000000000000000000001") {
		t.Error("an unlisted subject must not be staff")
	}
	if (&Config{}).IsStaffSubject("eth:0xdcb876ac74297655a73f04dc7a5039175dd15e10") {
		t.Error("an empty list must grant nobody")
	}
}
